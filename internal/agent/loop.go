package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"Pilot/internal/ai"
)

// Run 执行一次 Agent 请求。
// WARNING S级核心逻辑：理解状态变化、停止条件和失败回退后，应删除标记区域并独立重写。
// BEGIN S_LEVEL_REFERENCE
func (s *RunnerService) Run(ctx context.Context, request AgentRequest) (AgentResponse, error) {
	if s == nil || s.model == nil || s.registry == nil {
		return AgentResponse{}, ErrInvalidRunner
	}
	if err := request.Validate(); err != nil {
		return AgentResponse{}, err
	}
	policy := s.policy.Normalize()

	modelCtx, cancel := policy.Context(ctx)
	defer cancel()

	definitions, err := s.registry.Definitions(policy.AllowedTools)
	if err != nil {
		return AgentResponse{}, err
	}
	messages := append([]ai.Message(nil), request.History...)
	messages = append(messages, ai.Message{Role: ai.RoleUser, Content: request.Query})
	result := AgentResponse{}
	failedCalls := make(map[string]int)
	toolCalls := 0

	for step := 1; step <= policy.Execution.MaxSteps; step++ {
		if err := modelCtx.Err(); err != nil {
			return stopWithError(result, "agent context ended", err)
		}
		response, err := s.model.Generate(modelCtx, ai.ToolModelRequest{Messages: messages, Tools: definitions})
		if err != nil {
			return result, fmt.Errorf("generate agent step %d: %w", step, err)
		}
		result.Usage.InputTokens += response.Usage.InputTokens
		result.Usage.OutputTokens += response.Usage.OutputTokens

		if len(response.ToolCalls) == 0 {
			answer := strings.TrimSpace(response.Message.Content)
			if answer == "" {
				return result, ErrEmptyAgentAnswer
			}
			result.Answer = answer
			return result, nil
		}
		if content := strings.TrimSpace(response.Message.Content); content != "" {
			// 保留模型在工具调用前给出的文本，达到 fallback 时可以返回有限答案。
			result.Answer = content
		}
		messages = append(messages, response.Message)

		for _, call := range response.ToolCalls {
			if toolCalls >= policy.Budget.MaxToolCalls {
				return stopWithFallback(result, policy, ErrMaxToolCallsExceeded, "maximum tool calls reached")
			}
			toolCalls++
			audit := ToolCall{
				Name:      strings.TrimSpace(call.Name),
				Arguments: append(json.RawMessage(nil), call.Arguments...),
				Step:      step,
				Status:    ToolCallFailed,
			}
			if len(audit.Arguments) == 0 {
				audit.Arguments = json.RawMessage(`{}`)
			}
			if len(audit.Arguments) > policy.Budget.MaxArgumentsBytes || !json.Valid(audit.Arguments) {
				audit.Error = "tool arguments are invalid or too large"
				result.ToolCalls = append(result.ToolCalls, audit)
				return stopWithFallback(result, policy, ErrInvalidToolArguments, audit.Error)
			}
			if !policy.AllowsTool(audit.Name) {
				audit.Error = "tool is not allowed"
				result.ToolCalls = append(result.ToolCalls, audit)
				return stopWithFallback(result, policy, ErrToolNotAllowed, audit.Error)
			}
			tool, err := s.registry.Get(audit.Name)
			if err != nil {
				audit.Error = "tool is unavailable"
				result.ToolCalls = append(result.ToolCalls, audit)
				return stopWithFallback(result, policy, ErrToolUnavailable, audit.Error)
			}
			value, err := tool.Execute(modelCtx, audit.Arguments)
			if err != nil {
				audit.Error = "tool execution failed"
				result.ToolCalls = append(result.ToolCalls, audit)
				fingerprint := audit.Name + "\x00" + string(audit.Arguments)
				failedCalls[fingerprint]++
				if failedCalls[fingerprint] >= policy.Repeat.MaxConsecutiveFailures {
					return stopWithFallback(result, policy, ErrRepeatedToolFailure, "repeated tool failure")
				}
				if policy.Fallback.Mode == FallbackAnswerWithoutTool {
					if strings.TrimSpace(result.Answer) != "" {
						return stopWithFallback(result, policy, ErrToolExecutionFailed, audit.Error)
					}
					message, messageErr := toolMessage(call, nil, errors.New("tool execution failed"))
					if messageErr != nil {
						return result, fmt.Errorf("build failed tool result: %w", messageErr)
					}
					messages = append(messages, message)
					continue
				}
				return stopWithFallback(result, policy, ErrToolExecutionFailed, audit.Error)
			}
			audit.Status = ToolCallSucceeded
			result.ToolCalls = append(result.ToolCalls, audit)
			message, err := toolMessage(call, value, nil)
			if err != nil {
				return result, fmt.Errorf("build tool result: %w", err)
			}
			messages = append(messages, message)
		}
	}

	return stopWithFallback(result, policy, ErrMaxStepsExceeded, "maximum agent steps reached")
}

var (
	ErrEmptyAgentAnswer     = errors.New("agent returned an empty answer")
	ErrMaxStepsExceeded     = errors.New("maximum agent steps exceeded")
	ErrMaxToolCallsExceeded = errors.New("maximum agent tool calls exceeded")
	ErrInvalidToolArguments = errors.New("agent tool arguments are invalid")
	ErrToolNotAllowed       = errors.New("agent tool is not allowed")
	ErrToolUnavailable      = errors.New("agent tool is unavailable")
	ErrToolExecutionFailed  = errors.New("agent tool execution failed")
	ErrRepeatedToolFailure  = errors.New("agent repeated tool failure")
	ErrAgentContext         = errors.New("agent context ended")
)

func stopWithError(response AgentResponse, reason string, cause error) (AgentResponse, error) {
	response.Limitations = append(response.Limitations, reason)
	return response, fmt.Errorf("%w: %v", ErrAgentContext, cause)
}

func stopWithFallback(response AgentResponse, policy AgentPolicy, cause error, limitation string) (AgentResponse, error) {
	response.Limitations = append(response.Limitations, limitation)
	if policy.Fallback.Mode == FallbackAnswerWithoutTool && strings.TrimSpace(response.Answer) != "" {
		return response, nil
	}
	return response, cause
}

// END S_LEVEL_REFERENCE

// toolMessage 将结构化工具结果转换为模型上下文消息。
func toolMessage(call ai.ModelToolCall, result any, toolErr error) (ai.Message, error) {
	payload := struct {
		CallID string `json:"call_id"`
		Result any    `json:"result,omitempty"`
		Error  string `json:"error,omitempty"`
	}{CallID: call.ID, Result: result}
	if toolErr != nil {
		payload.Error = toolErr.Error()
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ai.Message{}, err
	}
	return ai.Message{Role: ai.RoleTool, Content: string(data)}, nil
}
