package eino

import (
	"context"
	"errors"
	"fmt"
	"strings"

	projectagent "Pilot/internal/agent"
	projectai "Pilot/internal/ai"
	"github.com/cloudwego/eino/compose"
	einoagent "github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

// EinoRunner 将 Eino ReAct Agent 适配为项目自己的 Runner 契约。
// ReAct 循环由 react.Agent.Generate 负责，这里不再额外嵌套 for 循环。
type EinoRunner struct {
	agent  *react.Agent
	policy projectagent.AgentPolicy
}

var _ projectagent.Runner = (*EinoRunner)(nil)

// NewEinoRunner 接收已经配置好模型、工具和 MaxStep 的 Eino Agent。
// 策略由组合根传入并校验，不从 AgentRequest 中读取。
func NewEinoRunner(einoAgent *react.Agent, policy projectagent.AgentPolicy) (*EinoRunner, error) {
	if einoAgent == nil {
		return nil, ErrInvalidEinoAgent
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &EinoRunner{agent: einoAgent, policy: policy}, nil
}

// Run 完成项目消息与 Eino 消息之间的转换，并调用 Eino ReAct。
func (r *EinoRunner) Run(ctx context.Context, request projectagent.AgentRequest) (projectagent.AgentResponse, error) {
	if r == nil || r.agent == nil {
		return projectagent.AgentResponse{}, ErrInvalidEinoAgent
	}
	if err := request.Validate(); err != nil {
		return projectagent.AgentResponse{}, err
	}
	runCtx, cancel := r.policy.Context(ctx)
	defer cancel()
	policyState := &policyRunState{}
	runCtx = withPolicyRunState(runCtx, policyState)
	messages, err := buildEinoMessages(request)
	if err != nil {
		return projectagent.AgentResponse{}, err
	}
	recorder := newToolAuditRecorder()
	response, err := r.agent.Generate(runCtx, messages, einoagent.WithComposeOptions(compose.WithCallbacks(recorder.callback())))
	if err != nil {
		return projectagent.AgentResponse{ToolCalls: mergeToolCalls(recorder.snapshot(), policyState.rejectedSnapshot())}, fmt.Errorf("generate with eino agent: %w", err)
	}
	if response == nil {
		return projectagent.AgentResponse{}, errors.New("eino agent returned empty response")
	}
	return projectagent.AgentResponse{
		Answer:    strings.TrimSpace(response.Content),
		Usage:     usageFromAgentResponse(response),
		ToolCalls: mergeToolCalls(recorder.snapshot(), policyState.rejectedSnapshot()),
	}, nil
}

func mergeToolCalls(executed, rejected []projectagent.ToolCall) []projectagent.ToolCall {
	if len(rejected) == 0 {
		return executed
	}
	result := make([]projectagent.ToolCall, 0, len(executed)+len(rejected))
	result = append(result, executed...)
	result = append(result, rejected...)
	return result
}

func usageFromAgentResponse(message *schema.Message) projectai.Usage {
	if message == nil || message.ResponseMeta == nil || message.ResponseMeta.Usage == nil {
		return projectai.Usage{}
	}
	usage := message.ResponseMeta.Usage
	return projectai.Usage{InputTokens: usage.PromptTokens, OutputTokens: usage.CompletionTokens}
}

// buildEinoMessages 把历史消息和当前用户问题按顺序转换给 ReAct。
func buildEinoMessages(request projectagent.AgentRequest) ([]*schema.Message, error) {
	messages := append([]projectai.Message(nil), request.History...)
	messages = append(messages, projectai.Message{Role: projectai.RoleUser, Content: request.Query})
	converted := make([]*schema.Message, 0, len(messages))
	for index, message := range messages {
		if !message.Role.Valid() {
			return nil, fmt.Errorf("message %d has invalid role %q", index, message.Role)
		}
		if strings.TrimSpace(message.Content) == "" {
			return nil, fmt.Errorf("message %d has empty content", index)
		}
		converted = append(converted, &schema.Message{Role: schema.RoleType(message.Role), Content: message.Content})
	}
	return converted, nil
}

var ErrInvalidEinoAgent = errors.New("eino agent is not initialized")
