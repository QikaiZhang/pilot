package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"Pilot/internal/ai"
	"Pilot/internal/memory"
)

// 三层流模型（理解本文件的关键）：
//
//	层1 ai.TokenStream  <-  eino / 供应商 SDK 的裸令牌流
//	     Recv() 只给「文本增量 + usage + finish_reason」，不关心落库、消息ID、取消。
//
//	层2 chat.EventStream <- 本文件定义的业务流（真正做语义归一化的那一层）
//	     Recv() 只返回 content / done / error 三种事件，并承担生命周期责任：
//	     累积完整 assistant、EOF 时才落库、context 取消关闭底层流、供应商错误映射成用户可读 code。
//
//	层3 SSE HTTP 输出    <-  internal/controller 的协议适配层
//	     把 StreamEvent 序列化成 data: {...}\n\n 并 Flush，不访问任何存储或 SDK。
//
//	Service.Stream 完成「层1 -> 层2」的转换；Controller 完成「层2 -> 层3」的转换。
//	真正的业务编排（LoadRecent -> 保存 user -> 启动模型流 -> EOF 保存 assistant）都在层2，
//	它保证传输层永远看不到 Redis/MySQL/供应商 SDK。
type serviceStream struct {
	ctx      context.Context
	upstream ai.TokenStream // 层1：裸令牌流，来自 eino/供应商
	memory   memoryStore
	request  TurnRequest
	newID    func() (string, error)
	now      func() time.Time
	content  strings.Builder // 累积所有 delta，EOF 时作为完整 assistant 内容
	usage    ai.Usage
	finished bool
	closed   bool
}

// Stream 启动一轮流式对话，返回层2业务流。
// 执行顺序与同步 Generate 保持一致：
//  1. LoadRecent 读历史窗口
//  2. 保存 user 消息（在启动模型前持久化）
//  3. 调用 s.model.Stream 拿到层1 TokenStream
//
// assistant 消息不在这里保存，由 serviceStream 在正常 EOF 时保存，避免保存半截回复。
func (s *Service) Stream(ctx context.Context, request TurnRequest) (EventStream, error) {
	normalized, err := normalizeTurn(request)
	if err != nil {
		return nil, err
	}
	request = normalized
	// 从 Redis 拿窗口记忆
	history, err := s.memory.LoadRecent(ctx, request.UserID, request.SessionID, s.config.HistoryLimit)
	if err != nil {
		return nil, fmt.Errorf("load recent conversation: %w", err)
	}

	userMessage, err := s.newMemoryMessage(request, memory.RoleUser, request.Query)
	if err != nil {
		return nil, err
	}
	// 先保存 user 消息，再启动模型流
	if err := s.memory.SaveMessages(ctx, []memory.ChatMessage{userMessage}); err != nil {
		return nil, fmt.Errorf("save user message: %w", err)
	}

	upstream, err := s.model.Stream(ctx, ai.ModelRequest{
		Messages: buildModelMessages(s.config.SystemPrompt, history, userMessage),
	})
	if err != nil {
		return nil, fmt.Errorf("start chat stream: %w", err)
	}

	return &serviceStream{
		ctx:      ctx,
		upstream: upstream,
		memory:   s.memory,
		request:  request,
		newID:    s.newID,
		now:      s.now,
	}, nil
}

// Recv 是「层1 -> 层2」转换的核心：每调用一次，要么返回一个业务事件，要么返回终止信号。
// 返回约定：
//   - (StreamEvent, nil)           一次可发送的应用事件（content / done / error）
//   - ({}, io.EOF)                 流已正常结束，调用方应停止读取
//   - ({}, context.Canceled/DeadlineExceeded)  请求被取消，行为等同于 EOF
//
// 业务不变式：空增量（Delta==""）跳过多发；上游非 EOF 错误映射为 StreamEventError，返回 error 时用户可读。
func (s *serviceStream) Recv() (StreamEvent, error) {
	if s == nil || s.upstream == nil {
		return StreamEvent{}, errors.New("chat stream is not initialized")
	}
	// 已结束或已关闭：后续一律返回 EOF，保证幂等
	if s.finished || s.closed {
		return StreamEvent{}, io.EOF
	}
	// 读取前先检查 context，客户端已断连则不再读底层流
	if err := s.ctx.Err(); err != nil {
		s.finished = true
		_ = s.Close()
		return StreamEvent{}, err
	}

	for {
		chunk, err := s.upstream.Recv()
		if err == nil {
			// usage 可能只出现在最后一个 chunk 上，收到就覆盖保存
			if chunk.Usage != nil {
				s.usage = *chunk.Usage
			}
			// 跳过空增量，不向客户端发送无意义事件
			if chunk.Delta == "" {
				continue
			}
			// 累积完整内容，同时作为独立增量下发
			s.content.WriteString(chunk.Delta)
			return StreamEvent{Type: StreamEventContent, Content: chunk.Delta}, nil
		}
		// 上游正常结束：此时才尝试保存 assistant（仍在 context 有效的前提下）
		if errors.Is(err, io.EOF) {
			return s.finish()
		}
		// 上游报错但 context 也取消了：优先按取消处理
		if ctxErr := s.ctx.Err(); ctxErr != nil {
			s.finished = true
			_ = s.Close()
			return StreamEvent{}, ctxErr
		}

		// 真正的供应商错误：不能保存半截回复，映射成用户可读的 error 事件并结束
		s.finished = true
		_ = s.Close()
		return StreamEvent{
			Type:  StreamEventError,
			Error: &StreamError{Code: "UPSTREAM_ERROR", Message: "stream receive failed"},
		}, nil
	}
}

// finish 处理层1正常 EOF：只有完整且非空时才保存 assistant，并返回一次 done。
// 之后的下一次 Recv 会返回 io.EOF（依赖 finished=true）。
func (s *serviceStream) finish() (StreamEvent, error) {
	s.finished = true
	// EOF 时若 context 已取消（如客户端刚断开），不保存回复
	if err := s.ctx.Err(); err != nil {
		_ = s.Close()
		return StreamEvent{}, err
	}
	// 模型什么都没说：不写半截/空 assistant，返回业务错误事件
	if strings.TrimSpace(s.content.String()) == "" {
		_ = s.Close()
		return StreamEvent{
			Type:  StreamEventError,
			Error: &StreamError{Code: "EMPTY_RESPONSE", Message: "chat model returned an empty response"},
		}, nil
	}

	// 生成 assistant 消息 ID 并持久化完整内容
	id, err := s.newID()
	if err != nil {
		_ = s.Close()
		return StreamEvent{}, fmt.Errorf("generate assistant message id: %w", err)
	}
	assistant := memory.ChatMessage{
		ID:        id,
		UserID:    s.request.UserID,
		SessionID: s.request.SessionID,
		Role:      memory.RoleAssistant,
		Content:   s.content.String(),
		CreatedAt: s.now(),
	}
	if err := s.memory.SaveMessages(s.ctx, []memory.ChatMessage{assistant}); err != nil {
		_ = s.Close()
		return StreamEvent{}, fmt.Errorf("save assistant message: %w", err)
	}

	// 保存成功后才关闭底层流；返回 done 携带最终用量
	_ = s.Close()
	usage := s.usage
	return StreamEvent{Type: StreamEventDone, Usage: &usage}, nil
}

// Close 关闭层1底层 TokenStream，必须幂等：重复调用或析构前已被 finish 关闭均不重复操作。
// 调用方（Controller 的 defer）与 serviceStream 内部都会调用，靠 closed 标志保证只真正关闭一次。
func (s *serviceStream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	if s.upstream == nil {
		return nil
	}
	return s.upstream.Close()
}
