package eino

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	projectagent "Pilot/internal/agent"

	"github.com/cloudwego/eino/compose"
)

// 策略拒绝不一定等于 Agent 执行失败；Runner 负责能力，runState 负责本次请求状态。
const (
	policyArgumentsTooLargeCode = "TOOL_ARGUMENTS_TOO_LARGE"
	policyToolCallsExceededCode = "TOOL_CALL_BUDGET_EXCEEDED"
	policyRepeatedFailureCode   = "TOOL_REPEATED_FAILURE"
)

// policyRunState 保存一次 Agent Run 内由策略中间件产生的状态。
// 它不能放在 EinoRunner 实例上，否则并发请求会共享预算和审计记录。
type policyRunState struct {
	mu        sync.Mutex
	step      int
	toolCalls int
	failures  map[string]int
	rejected  []projectagent.ToolCall
}

type policyStateKey struct{}

func withPolicyRunState(ctx context.Context, state *policyRunState) context.Context {
	return context.WithValue(ctx, policyStateKey{}, state)
}

func policyRunStateFrom(ctx context.Context) *policyRunState {
	state, _ := ctx.Value(policyStateKey{}).(*policyRunState)
	return state
}

func (s *policyRunState) setStep(step int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.step = step
	s.mu.Unlock()
}

func (s *policyRunState) reserveToolCall(max int) bool {
	if s == nil || max <= 0 {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.toolCalls >= max {
		return false
	}
	s.toolCalls++
	return true
}

func (s *policyRunState) repeatedFailure(key string, max int) bool {
	if s == nil || max <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failures[key] >= max
}

func (s *policyRunState) recordToolResult(key string, err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failures == nil {
		s.failures = make(map[string]int)
	}
	if err == nil {
		delete(s.failures, key)
		return
	}
	s.failures[key]++
}

func (s *policyRunState) rejectTool(name string, step int, arguments, message string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if step <= 0 {
		step = s.step
	}
	s.rejected = append(s.rejected, projectagent.ToolCall{
		Name: name, Arguments: []byte(arguments), Step: step, Status: projectagent.ToolCallRejected, Error: message,
	})
	s.mu.Unlock()
}

func (s *policyRunState) rejectedSnapshot() []projectagent.ToolCall {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]projectagent.ToolCall(nil), s.rejected...)
}

// newPolicyMiddleware 创建 Eino 工具策略中间件。
// 当前处理调用预算和参数大小这两类本地策略拒绝；它返回正常 ToolOutput，
// 让 ReAct 可以把拒绝原因交回模型，而不是用 Context 取消打断 Graph。
func newPolicyMiddleware(policy projectagent.AgentPolicy) compose.ToolMiddleware {
	policy = policy.Normalize()
	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				name := "unknown"
				arguments := "{}"
				if input != nil {
					if input.Name != "" {
						name = input.Name
					}
					if input.Arguments != "" {
						arguments = input.Arguments
					}
				}
				state := policyRunStateFrom(ctx)
				if state != nil && !state.reserveToolCall(policy.Budget.MaxToolCalls) {
					const message = "tool call budget exceeded"
					state.rejectTool(name, 0, arguments, message)
					return &compose.ToolOutput{Result: policyRejectionResult(policyToolCallsExceededCode, message)}, nil
				}
				maxBytes := policy.Budget.MaxArgumentsBytes
				if input != nil && maxBytes > 0 && len(input.Arguments) > maxBytes {
					const message = "tool arguments exceed policy limit"
					if state := policyRunStateFrom(ctx); state != nil {
						state.rejectTool(name, 0, arguments, message)
					}
					return &compose.ToolOutput{Result: policyRejectionResult(policyArgumentsTooLargeCode, message)}, nil
				}
				fingerprint := toolCallFingerprint(name, arguments)
				if state != nil && state.repeatedFailure(fingerprint, policy.Repeat.MaxConsecutiveFailures) {
					const message = "same tool call reached repeated failure limit"
					state.rejectTool(name, 0, arguments, message)
					return &compose.ToolOutput{Result: policyRejectionResult(policyRepeatedFailureCode, message)}, nil
				}
				output, err := next(ctx, input)
				if state != nil {
					state.recordToolResult(fingerprint, err)
				}
				return output, err
			}
		},
	}
}

// toolCallFingerprint 只用于本次 Run 内的重复调用保护，不作为跨请求幂等键。
// JSON 对象经 encoding/json 重编码后按稳定键顺序输出；数组顺序保持不变。
func toolCallFingerprint(name, arguments string) string {
	canonical := arguments
	var value any
	if err := json.Unmarshal([]byte(arguments), &value); err == nil {
		if data, marshalErr := json.Marshal(value); marshalErr == nil {
			canonical = string(data)
		}
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s", name, canonical)))
	return string(sum[:])
}

func policyRejectionResult(code, message string) string {
	payload := struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{OK: false}
	payload.Error.Code = code
	payload.Error.Message = message
	data, _ := json.Marshal(payload)
	return string(data)
}
