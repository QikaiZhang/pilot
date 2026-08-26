package chat

import "Pilot/internal/ai"

// TurnRequest 描述用户的一轮对话请求。
// 模型参数由服务端 AI 配置管理，不由调用方直接控制。
type TurnRequest struct {
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
	Query     string `json:"query"`
}

// ChatResponse 是同步对话的一次完整结果。
type ChatResponse struct {
	Content string   `json:"content"`
	Usage   ai.Usage `json:"usage"`
}

// StreamEventType 描述应用层的流式协议事件类型。
type StreamEventType string

const (
	StreamEventContent StreamEventType = "content"
	StreamEventDone    StreamEventType = "done"
	StreamEventError   StreamEventType = "error"
)

// StreamError 可以安全地通过 JSON/SSE 暴露给客户端。
// 供应商特有错误必须先映射，再进入这个契约。
type StreamError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// StreamEvent 是发送给聊天客户端的协议事件。
// done 事件可以携带最终 token 用量，content 事件只携带文本增量。
type StreamEvent struct {
	Type    StreamEventType `json:"type"`
	Content string          `json:"content,omitempty"`
	Usage   *ai.Usage       `json:"usage,omitempty"`
	Error   *StreamError    `json:"error,omitempty"`
}

// EventStream 是聊天层的流，由 HTTP/SSE 适配器消费。
type EventStream interface {
	Recv() (StreamEvent, error)
	Close() error
}
