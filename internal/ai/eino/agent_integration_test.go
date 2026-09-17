package eino

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	projectagent "Pilot/internal/agent"
	projecttools "Pilot/internal/tools"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type scriptedToolCallingModel struct {
	responses []*schema.Message
	inputs    [][]*schema.Message
}

type contextEndingModel struct{}

func (contextEndingModel) Generate(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (contextEndingModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{}), nil
}

func (m contextEndingModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func (m *scriptedToolCallingModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.inputs = append(m.inputs, input)
	response := m.responses[0]
	m.responses = m.responses[1:]
	return response, nil
}

func (m *scriptedToolCallingModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{}), nil
}

func (m *scriptedToolCallingModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

type integrationTool struct {
	calls int
}

func (*integrationTool) Name() string { return "health_check" }

func (*integrationTool) Description() string { return "check dependencies" }

func (*integrationTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (t *integrationTool) Execute(context.Context, json.RawMessage) (any, error) {
	t.calls++
	return map[string]string{"status": "up"}, nil
}

func TestEinoRunnerRecordsToolCallFromReActExecution(t *testing.T) {
	tool := &integrationTool{}
	registry, err := projecttools.NewRegistry(tool)
	if err != nil {
		t.Fatal(err)
	}
	policy := projectagent.AgentPolicy{
		Execution:    projectagent.ExecutionPolicy{MaxSteps: 3, Timeout: time.Second},
		Budget:       projectagent.BudgetPolicy{MaxToolCalls: 4, MaxArgumentsBytes: 1024},
		AllowedTools: []string{"health_check"},
		Fallback:     projectagent.FallbackPolicy{Mode: projectagent.FallbackReturnError},
	}
	model := &scriptedToolCallingModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "health_check",
				Arguments: `{}`,
			},
		}}),
		schema.AssistantMessage("依赖正常", nil),
	}}
	einoAgent, err := NewReActAgent(context.Background(), model, registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewEinoRunner(einoAgent, policy)
	if err != nil {
		t.Fatal(err)
	}

	response, err := runner.Run(context.Background(), projectagent.AgentRequest{
		UserID: "u1", SessionID: "s1", Query: "检查依赖",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if response.Answer != "依赖正常" || tool.calls != 1 {
		t.Fatalf("response = %+v, tool calls = %d", response, tool.calls)
	}
	if len(response.ToolCalls) != 1 {
		t.Fatalf("tool audit = %+v, want one record", response.ToolCalls)
	}
	audit := response.ToolCalls[0]
	if audit.Name != "health_check" || audit.Step != 1 || audit.Status != projectagent.ToolCallSucceeded {
		t.Fatalf("audit = %+v", audit)
	}
	if strings.TrimSpace(string(audit.Arguments)) != "{}" {
		t.Fatalf("audit arguments = %q", audit.Arguments)
	}
	if len(model.inputs) != 2 || len(model.inputs[1]) < 2 || model.inputs[1][len(model.inputs[1])-1].Role != schema.Tool {
		t.Fatalf("model inputs = %+v, want second request to contain tool observation", model.inputs)
	}
}

func TestEinoReActSoftRejectsOversizedToolArguments(t *testing.T) {
	tool := &integrationTool{}
	registry, err := projecttools.NewRegistry(tool)
	if err != nil {
		t.Fatal(err)
	}
	policy := projectagent.AgentPolicy{
		Execution:    projectagent.ExecutionPolicy{MaxSteps: 3, Timeout: time.Second},
		Budget:       projectagent.BudgetPolicy{MaxToolCalls: 4, MaxArgumentsBytes: 4},
		AllowedTools: []string{"health_check"},
		Fallback:     projectagent.FallbackPolicy{Mode: projectagent.FallbackReturnError},
	}
	model := &scriptedToolCallingModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:       "call-oversized",
			Function: schema.FunctionCall{Name: "health_check", Arguments: `{"too":"large"}`},
		}}),
		schema.AssistantMessage("参数已被策略拒绝", nil),
	}}
	einoAgent, err := NewReActAgent(context.Background(), model, registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewEinoRunner(einoAgent, policy)
	if err != nil {
		t.Fatal(err)
	}
	response, err := runner.Run(context.Background(), projectagent.AgentRequest{UserID: "u1", SessionID: "s1", Query: "检查依赖"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if response.Answer != "参数已被策略拒绝" || tool.calls != 0 {
		t.Fatalf("response = %+v, tool calls = %d", response, tool.calls)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Status != projectagent.ToolCallRejected {
		t.Fatalf("tool audit = %+v, want one rejected call", response.ToolCalls)
	}
}

func TestEinoRunnerMapsPolicyTimeoutToStableError(t *testing.T) {
	registry, err := projecttools.NewRegistry(&integrationTool{})
	if err != nil {
		t.Fatal(err)
	}
	policy := projectagent.AgentPolicy{
		Execution:    projectagent.ExecutionPolicy{MaxSteps: 2, Timeout: 10 * time.Millisecond},
		Budget:       projectagent.BudgetPolicy{MaxToolCalls: 2, MaxArgumentsBytes: 100},
		AllowedTools: []string{"health_check"},
		Fallback:     projectagent.FallbackPolicy{Mode: projectagent.FallbackReturnError},
	}
	einoAgent, err := NewReActAgent(context.Background(), contextEndingModel{}, registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewEinoRunner(einoAgent, policy)
	if err != nil {
		t.Fatal(err)
	}
	response, err := runner.Run(context.Background(), projectagent.AgentRequest{UserID: "u1", SessionID: "s1", Query: "检查依赖"})
	if !errors.Is(err, projectagent.ErrAgentTimeout) {
		t.Fatalf("Run() error = %v, want ErrAgentTimeout", err)
	}
	if len(response.Limitations) != 1 || response.Limitations[0] != "agent execution timed out" {
		t.Fatalf("limitations = %+v", response.Limitations)
	}
}

func TestEinoRunnerPreservesParentCancellation(t *testing.T) {
	registry, err := projecttools.NewRegistry(&integrationTool{})
	if err != nil {
		t.Fatal(err)
	}
	policy := projectagent.AgentPolicy{
		Execution:    projectagent.ExecutionPolicy{MaxSteps: 2, Timeout: time.Second},
		Budget:       projectagent.BudgetPolicy{MaxToolCalls: 2, MaxArgumentsBytes: 100},
		AllowedTools: []string{"health_check"},
		Fallback:     projectagent.FallbackPolicy{Mode: projectagent.FallbackReturnError},
	}
	einoAgent, err := NewReActAgent(context.Background(), contextEndingModel{}, registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewEinoRunner(einoAgent, policy)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response, err := runner.Run(ctx, projectagent.AgentRequest{UserID: "u1", SessionID: "s1", Query: "检查依赖"})
	if !errors.Is(err, projectagent.ErrAgentCancelled) {
		t.Fatalf("Run() error = %v, want ErrAgentCancelled", err)
	}
	if len(response.Limitations) != 1 || response.Limitations[0] != "agent execution was cancelled" {
		t.Fatalf("limitations = %+v", response.Limitations)
	}
}
