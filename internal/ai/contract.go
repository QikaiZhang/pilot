package ai

import (
	"context"
	"encoding/json"
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

// ToolDefinition 描述模型可以选择调用的工具。
// Parameters 是 JSON Schema，保持供应商无关，适配器再转换为 Eino 或其他 SDK 类型。
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ModelToolCall 是模型生成的一次工具调用意图，不包含执行状态。
// ID 用于把后续 ToolResult 与本次调用关联起来。
type ModelToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult 是工具执行后的观察结果，Agent 会把它转换为 RoleTool 消息交回模型。
type ToolResult struct {
	CallID  string `json:"call_id"`
	Name    string `json:"name"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ToolModelRequest 是支持工具调用的模型请求。
type ToolModelRequest struct {
	Messages []Message        `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
	Options  ModelOptions     `json:"options"`
}

// ToolModelResponse 是支持工具调用的模型响应。
// ToolCalls 非空时，Message 通常是工具调用前的 assistant 消息。
type ToolModelResponse struct {
	Message      Message         `json:"message"`
	ToolCalls    []ModelToolCall `json:"tool_calls,omitempty"`
	Usage        Usage           `json:"usage"`
	FinishReason string          `json:"finish_reason,omitempty"`
}

// ToolCallingChatModel 是具备工具调用能力的模型契约。
// 它与 ChatModel 分开，避免普通聊天调用被迫理解工具协议。
type ToolCallingChatModel interface {
	Generate(ctx context.Context, request ToolModelRequest) (ToolModelResponse, error)
}

func IsRoleValid(r string) error {
	r = strings.TrimSpace(r)
	if r == string(RoleAssistant) || r == string(RoleSystem) || r == string(RoleTool) || r == string(RoleUser) {
		return nil
	}
	return errors.New("role is invalid")
}

// Chunk 是检索返回的一个文档片段，来自知识库切分。
// DocID 标识源文档，ChunkID 唯一标识片段；其余字段用于精确过滤与追溯。
type Chunk struct {
	DocID    string   `json:"doc_id"`
	ChunkID  string   `json:"chunk_id"`
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Source   string   `json:"source,omitempty"`
	Category string   `json:"category,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Version  int      `json:"version,omitempty"`
	Score    float64  `json:"score"`
}

// Embedder 把文本编码为定长向量。索引侧与查询侧必须使用同一个 Embedder，
// 否则向量维度或语义不一致，检索失效。
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dim() int
}

// Retriever 对单个 query 做混合召回（关键词 + 向量），并返回按相关度降序的片段。
// 空召回返回空切片而非 error；只有 ES 不可用、超时或 Embedding 失败才返回 error。
type Retriever interface {
	Retrieve(ctx context.Context, query string, topK int) ([]Chunk, error)
}
