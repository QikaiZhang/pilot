// Package agent 定义 Agent 编排层的项目内契约。
// 这里描述输入、执行策略和可审计输出，不直接暴露 Eino 或具体工具实现。
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"Pilot/internal/ai"
)

// AgentRequest 是一次 Agent 执行的业务输入。
// 历史消息由上层会话服务准备，Agent 不直接访问 Redis 或 MySQL。
type AgentRequest struct {
	UserID    string       `json:"user_id"`
	SessionID string       `json:"session_id"`
	Query     string       `json:"query"`
	History   []ai.Message `json:"history,omitempty"`
}

// AgentResponse 是 Agent 完成一次执行后的结果。
// 会话标识和原始 query 已由上层持有，因此这里只返回 Agent 产生的结果。
type AgentResponse struct {
	Answer      string     `json:"answer"`
	ToolCalls   []ToolCall `json:"tool_calls,omitempty"`
	Usage       ai.Usage   `json:"usage"`
	Limitations []string   `json:"limitations,omitempty"`
}

// ToolCall 是一次工具执行记录。
// Step 表示第几轮 Agent 决策，不是模型 token 数，也不是工具内部步骤。
type ToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Step      int             `json:"step"`
	Status    ToolCallStatus  `json:"status"`
	Error     string          `json:"error,omitempty"`
}

// ToolCallStatus 表示工具执行结果。
type ToolCallStatus string

const (
	ToolCallSucceeded ToolCallStatus = "succeeded"
	ToolCallFailed    ToolCallStatus = "failed"
	ToolCallRejected  ToolCallStatus = "rejected"
)

// FallbackMode 描述工具或模型规划失败后的处理策略。
// 它是策略值，不是函数；具体动作由请求级 Runner（PolicyAwareRunner）执行。
type FallbackMode string

const (
	FallbackAnswerWithoutTool FallbackMode = "answer_without_tool"
	FallbackReturnError       FallbackMode = "return_error"
)

// ExecutionPolicy 控制一次 Agent 执行的时间和决策轮数。
type ExecutionPolicy struct {
	MaxSteps int
	Timeout  time.Duration
}

// BudgetPolicy 控制一次 Agent 执行可以消耗的工具资源。
type BudgetPolicy struct {
	MaxToolCalls      int
	MaxArgumentsBytes int
}

// RepeatCallPolicy 控制相同工具和参数连续失败时的停止阈值。
// 它是单次 Run 内的循环保护，不等同于跨请求共享的 Circuit Breaker。
type RepeatCallPolicy struct {
	MaxConsecutiveFailures int
}

// FallbackPolicy 描述策略拒绝或工具失败后的处理方式。
type FallbackPolicy struct {
	Mode FallbackMode
}

// AgentPolicy 是服务端控制的 Agent 执行策略。
// 不应直接从普通用户请求中读取，避免用户放大循环次数和调用成本。
type AgentPolicy struct {
	Execution    ExecutionPolicy
	Budget       BudgetPolicy
	Repeat       RepeatCallPolicy
	Fallback     FallbackPolicy
	AllowedTools []string
}

// Runner 是聊天业务层依赖的 Agent 能力。
type Runner interface {
	Run(ctx context.Context, request AgentRequest) (AgentResponse, error)
}

var (
	ErrInvalidRequest = errors.New("agent request is invalid")
	ErrInvalidPolicy  = errors.New("agent policy is invalid")
	ErrAgentTimeout   = errors.New("agent execution timed out")
	ErrAgentCancelled = errors.New("agent execution was cancelled")
)

// Validate 检查 AgentRequest 的最小业务约束。
func (r AgentRequest) Validate() error {
	if strings.TrimSpace(r.UserID) == "" || strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.Query) == "" {
		return ErrInvalidRequest
	}
	return nil
}

// Validate 检查服务端 Agent 策略，避免循环无上限或执行时间失控。
func (p AgentPolicy) Validate() error {
	if p.Execution.MaxSteps <= 0 || p.Execution.Timeout <= 0 {
		return ErrInvalidPolicy
	}
	if p.Budget.MaxToolCalls < 0 || p.Budget.MaxArgumentsBytes < 0 {
		return ErrInvalidPolicy
	}
	if p.Repeat.MaxConsecutiveFailures < 0 {
		return ErrInvalidPolicy
	}
	if p.Fallback.Mode != FallbackAnswerWithoutTool && p.Fallback.Mode != FallbackReturnError {
		return ErrInvalidPolicy
	}
	seen := make(map[string]struct{}, len(p.AllowedTools))
	for _, name := range p.AllowedTools {
		name = strings.TrimSpace(name)
		if name == "" {
			return ErrInvalidPolicy
		}
		if _, exists := seen[name]; exists {
			return ErrInvalidPolicy
		}
		seen[name] = struct{}{}
	}
	return nil
}

const (
	defaultMaxToolCalls           = 16
	defaultMaxArgumentsBytes      = 64 << 10
	defaultMaxConsecutiveFailures = 2
)

// Normalize fills optional policy limits with service defaults.
// Required execution fields are still checked by Validate.
func (p AgentPolicy) Normalize() AgentPolicy {
	if p.Budget.MaxToolCalls == 0 {
		p.Budget.MaxToolCalls = defaultMaxToolCalls
	}
	if p.Budget.MaxArgumentsBytes == 0 {
		p.Budget.MaxArgumentsBytes = defaultMaxArgumentsBytes
	}
	if p.Repeat.MaxConsecutiveFailures == 0 {
		p.Repeat.MaxConsecutiveFailures = defaultMaxConsecutiveFailures
	}
	if p.Repeat.MaxConsecutiveFailures == 0 {
		p.Repeat.MaxConsecutiveFailures = defaultMaxConsecutiveFailures
	}
	return p
}

// ResolveFallback 在请求级 Runner 执行失败后按 FallbackPolicy 收口。
// answer_without_tool 只在模型已产出非空文本时降级为有限回答，并保留已完成的工具审计；
// return_error 始终返回原始错误。两种模式都会把失败原因写入 limitations。
func (p AgentPolicy) ResolveFallback(response AgentResponse, cause error, limitation string) (AgentResponse, error) {
	response.Limitations = append(response.Limitations, limitation)
	if p.Fallback.Mode == FallbackAnswerWithoutTool && strings.TrimSpace(response.Answer) != "" {
		return response, nil
	}
	return response, cause
}

// AllowsTool 判断策略是否允许调用指定工具；空白值一律拒绝。
func (p AgentPolicy) AllowsTool(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, allowed := range p.AllowedTools {
		if strings.TrimSpace(allowed) == name {
			return true
		}
	}
	return false
}

// Context 从上层 context 派生本次 Agent 执行的策略超时，并保留父 context 的取消信号。
func (p AgentPolicy) Context(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, p.Execution.Timeout)
}
