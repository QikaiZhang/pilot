package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"Pilot/internal/ai"
	projecttools "Pilot/internal/tools"
)

type scriptedModel struct {
	responses []ai.ToolModelResponse
	requests  []ai.ToolModelRequest
}

func (m *scriptedModel) Generate(_ context.Context, request ai.ToolModelRequest) (ai.ToolModelResponse, error) {
	m.requests = append(m.requests, request)
	if len(m.responses) == 0 {
		return ai.ToolModelResponse{}, errors.New("unexpected model call")
	}
	response := m.responses[0]
	m.responses = m.responses[1:]
	return response, nil
}

type loopTool struct {
	name   string
	result any
	err    error
	calls  int
}

func (t *loopTool) Name() string              { return t.name }
func (*loopTool) Description() string         { return "loop test tool" }
func (*loopTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *loopTool) Execute(context.Context, json.RawMessage) (any, error) {
	t.calls++
	return t.result, t.err
}

func newLoopRunner(t *testing.T, model ai.ToolCallingChatModel, tool projecttools.Tool, fallback FallbackMode) *RunnerService {
	t.Helper()
	registry, err := projecttools.NewRegistry(tool)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(model, registry, AgentPolicy{
		Execution:    ExecutionPolicy{MaxSteps: 3, Timeout: time.Second},
		AllowedTools: []string{tool.Name()},
		Fallback:     FallbackPolicy{Mode: fallback},
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func TestRunnerServiceExecutesToolThenReturnsFinalAnswer(t *testing.T) {
	tool := &loopTool{name: "health_check", result: map[string]string{"status": "ok"}}
	model := &scriptedModel{responses: []ai.ToolModelResponse{
		{Message: ai.Message{Role: ai.RoleAssistant}, ToolCalls: []ai.ModelToolCall{{ID: "call-1", Name: "health_check", Arguments: json.RawMessage(`{}`)}}, Usage: ai.Usage{InputTokens: 10}},
		{Message: ai.Message{Role: ai.RoleAssistant, Content: "依赖正常"}, Usage: ai.Usage{OutputTokens: 4}},
	}}

	response, err := newLoopRunner(t, model, tool, FallbackReturnError).Run(context.Background(), AgentRequest{UserID: "u1", SessionID: "s1", Query: "检查依赖"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if response.Answer != "依赖正常" || len(response.ToolCalls) != 1 || response.ToolCalls[0].Status != ToolCallSucceeded {
		t.Fatalf("response = %+v", response)
	}
	if tool.calls != 1 || len(model.requests) != 2 {
		t.Fatalf("tool calls = %d, model requests = %d", tool.calls, len(model.requests))
	}
	if len(model.requests[1].Messages) != 3 || model.requests[1].Messages[2].Role != ai.RoleTool {
		t.Fatalf("second request messages = %+v, want tool observation", model.requests[1].Messages)
	}
	if response.Usage.InputTokens != 10 || response.Usage.OutputTokens != 4 {
		t.Fatalf("usage = %+v", response.Usage)
	}
}

func TestRunnerServiceStopsOnToolFailure(t *testing.T) {
	tool := &loopTool{name: "health_check", err: errors.New("connection refused")}
	model := &scriptedModel{responses: []ai.ToolModelResponse{{
		ToolCalls: []ai.ModelToolCall{{Name: "health_check", Arguments: json.RawMessage(`{}`)}},
	}}}

	response, err := newLoopRunner(t, model, tool, FallbackReturnError).Run(context.Background(), AgentRequest{UserID: "u1", SessionID: "s1", Query: "检查依赖"})
	if !errors.Is(err, ErrToolExecutionFailed) {
		t.Fatalf("Run() error = %v, want %v", err, ErrToolExecutionFailed)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Status != ToolCallFailed || tool.calls != 1 {
		t.Fatalf("response = %+v, calls = %d", response, tool.calls)
	}
}

func TestRunnerServiceUsesLimitedAnswerOnToolFailure(t *testing.T) {
	tool := &loopTool{name: "health_check", err: errors.New("temporary failure")}
	model := &scriptedModel{responses: []ai.ToolModelResponse{{
		Message:   ai.Message{Role: ai.RoleAssistant, Content: "我无法确认依赖状态"},
		ToolCalls: []ai.ModelToolCall{{Name: "health_check", Arguments: json.RawMessage(`{}`)}},
	}}}

	response, err := newLoopRunner(t, model, tool, FallbackAnswerWithoutTool).Run(context.Background(), AgentRequest{UserID: "u1", SessionID: "s1", Query: "检查依赖"})
	if err != nil || response.Answer != "我无法确认依赖状态" || len(response.Limitations) != 1 {
		t.Fatalf("response = %+v, error = %v", response, err)
	}
}

func TestRunnerServiceStopsRepeatedToolFailure(t *testing.T) {
	tool := &loopTool{name: "health_check", err: errors.New("down")}
	model := &scriptedModel{responses: []ai.ToolModelResponse{
		{ToolCalls: []ai.ModelToolCall{{Name: "health_check", Arguments: json.RawMessage(`{"x":1}`)}}},
		{ToolCalls: []ai.ModelToolCall{{Name: "health_check", Arguments: json.RawMessage(`{"x":1}`)}}},
	}}
	_, err := newLoopRunner(t, model, tool, FallbackAnswerWithoutTool).Run(context.Background(), AgentRequest{UserID: "u1", SessionID: "s1", Query: "检查依赖"})
	if !errors.Is(err, ErrRepeatedToolFailure) {
		t.Fatalf("Run() error = %v, want repeated failure stop", err)
	}
	if tool.calls != 2 {
		t.Fatalf("tool calls = %d, want 2 before repeated failure stop", tool.calls)
	}
}

func TestRunnerServiceEnforcesStepAndToolCallBudgets(t *testing.T) {
	t.Run("max steps", func(t *testing.T) {
		tool := &loopTool{name: "health_check", result: map[string]string{"status": "ok"}}
		model := &scriptedModel{responses: []ai.ToolModelResponse{{
			ToolCalls: []ai.ModelToolCall{{Name: "health_check", Arguments: json.RawMessage(`{}`)}},
		}}}
		runner := newLoopRunner(t, model, tool, FallbackReturnError)
		runner.policy.Execution.MaxSteps = 1

		_, err := runner.Run(context.Background(), AgentRequest{UserID: "u1", SessionID: "s1", Query: "检查依赖"})
		if !errors.Is(err, ErrMaxStepsExceeded) || tool.calls != 1 {
			t.Fatalf("error = %v, calls = %d", err, tool.calls)
		}
	})

	t.Run("max tool calls", func(t *testing.T) {
		tool := &loopTool{name: "health_check", result: map[string]string{"status": "ok"}}
		model := &scriptedModel{responses: []ai.ToolModelResponse{{
			ToolCalls: []ai.ModelToolCall{
				{Name: "health_check", Arguments: json.RawMessage(`{}`)},
				{Name: "health_check", Arguments: json.RawMessage(`{}`)},
			},
		}}}
		runner := newLoopRunner(t, model, tool, FallbackReturnError)
		runner.policy.Budget.MaxToolCalls = 1

		_, err := runner.Run(context.Background(), AgentRequest{UserID: "u1", SessionID: "s1", Query: "检查依赖"})
		if !errors.Is(err, ErrMaxToolCallsExceeded) || tool.calls != 1 {
			t.Fatalf("error = %v, calls = %d", err, tool.calls)
		}
	})
}
