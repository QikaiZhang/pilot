package ai

import (
	"context"
	"errors"
	"strings"
)

// Role 表示模型理解的消息角色，也包含 Agent 流程中的工具消息。
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

func (r Role) Valid() bool {
	switch r {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		return true
	default:
		return false
	}
}

// Message 表示提供给模型的上下文消息。
// 它不包含持久化 ID、用户 ID、会话 ID 或数据库时间戳。
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// ModelRequest 是传给 AI 模型的供应商无关请求。
type ModelRequest struct {
	Messages []Message    `json:"messages"`
	Options  ModelOptions `json:"options"`
}

// ModelOptions 由服务端策略和配置决定，不直接暴露给普通用户请求。
type ModelOptions struct {
	Model       string  `json:"model,omitempty"`
	Temperature float32 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
}

// Usage 保存供应商无关的 token 用量统计。
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ModelResponse 是模型返回的完整结果。
type ModelResponse struct {
	Message Message `json:"message"`
	Usage   Usage   `json:"usage"`
}

// ModelChunk 是一次供应商无关的流式结果。
// 流结束由 TokenStream 返回 io.EOF 表示，不额外伪造一个结束 chunk。
type ModelChunk struct {
	Delta        string
	Usage        *Usage
	FinishReason string
}

// TokenStream 负责模型流的生命周期，调用方必须在使用完成后关闭它。
type TokenStream interface {
	Recv() (ModelChunk, error)
	Close() error
}

// ChatModel 是聊天业务所依赖的唯一模型能力接口。
// Eino 和具体供应商 SDK 类型都应该隐藏在实现该接口的适配器后面。
type ChatModel interface {
	Generate(ctx context.Context, request ModelRequest) (ModelResponse, error)
	Stream(ctx context.Context, request ModelRequest) (TokenStream, error)
}

func IsRoleValid(r string) error {
	r = strings.TrimSpace(r)
	if r == string(RoleAssistant) || r == string(RoleSystem) || r == string(RoleTool) || r == string(RoleUser) {
		return nil
	}
	return errors.New("role is invalid")
}
