package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type fakePolicyRunner struct {
	run func(ctx context.Context, request AgentRequest) (AgentResponse, error)
}

func (f *fakePolicyRunner) Run(ctx context.Context, request AgentRequest) (AgentResponse, error) {
	return f.run(ctx, request)
}

func policyRunnerTestPolicy(fallback FallbackMode) AgentPolicy {
	return AgentPolicy{
		Execution:    ExecutionPolicy{MaxSteps: 3, Timeout: time.Second},
		Budget:       BudgetPolicy{MaxToolCalls: 4, MaxArgumentsBytes: 1024},
		Repeat:       RepeatCallPolicy{MaxConsecutiveFailures: 2},
		AllowedTools: []string{"health_check"},
		Fallback:     FallbackPolicy{Mode: fallback},
	}
}

func policyRunnerTestRequest() AgentRequest {
	return AgentRequest{UserID: "u1", SessionID: "s1", Query: "检查依赖"}
}

func TestPolicyAwareRunnerPassesThroughSuccessfulRun(t *testing.T) {
	runner, err := NewPolicyAwareRunner(&fakePolicyRunner{run: func(ctx context.Context, _ AgentRequest) (AgentResponse, error) {
		if ctx.Err() != nil {
			t.Fatalf("run context should stay live, err = %v", ctx.Err())
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatalf("run context should carry the policy deadline")
		}
		return AgentResponse{Answer: "依赖正常"}, nil
	}}, policyRunnerTestPolicy(FallbackReturnError))
	if err != nil {
		t.Fatal(err)
	}
	response, err := runner.Run(context.Background(), policyRunnerTestRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if response.Answer != "依赖正常" {
		t.Fatalf("response = %+v", response)
	}
}

func TestPolicyAwareRunnerClassifiesWrappedDeadlineAsTimeout(t *testing.T) {
	runner, err := NewPolicyAwareRunner(&fakePolicyRunner{run: func(ctx context.Context, _ AgentRequest) (AgentResponse, error) {
		return AgentResponse{ToolCalls: []ToolCall{{Name: "health_check", Status: ToolCallFailed}}},
			fmt.Errorf("generate with eino agent: %w", context.DeadlineExceeded)
	}}, policyRunnerTestPolicy(FallbackReturnError))
	if err != nil {
		t.Fatal(err)
	}
	response, err := runner.Run(context.Background(), policyRunnerTestRequest())
	if !errors.Is(err, ErrAgentTimeout) {
		t.Fatalf("Run() error = %v, want ErrAgentTimeout", err)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Name != "health_check" {
		t.Fatalf("audit records should survive the timeout, response = %+v", response)
	}
	if len(response.Limitations) != 1 || response.Limitations[0] != "agent execution timed out" {
		t.Fatalf("limitations = %v", response.Limitations)
	}
}

func TestPolicyAwareRunnerClassifiesDeadlineExceededWithoutWrappedError(t *testing.T) {
	// 内部 Runner 透传普通依赖错误，但 runCtx 的 deadline 已经到期：
	// 生命周期语义优先于普通错误。
	policy := policyRunnerTestPolicy(FallbackReturnError)
	policy.Execution.Timeout = 20 * time.Millisecond
	runner, err := NewPolicyAwareRunner(&fakePolicyRunner{run: func(ctx context.Context, _ AgentRequest) (AgentResponse, error) {
		<-ctx.Done()
		return AgentResponse{}, errors.New("es connection refused")
	}}, policy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runner.Run(context.Background(), policyRunnerTestRequest())
	if !errors.Is(err, ErrAgentTimeout) {
		t.Fatalf("Run() error = %v, want ErrAgentTimeout", err)
	}
}

func TestPolicyAwareRunnerClassifiesCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	runner, err := NewPolicyAwareRunner(&fakePolicyRunner{run: func(ctx context.Context, _ AgentRequest) (AgentResponse, error) {
		cancel()
		<-ctx.Done()
		return AgentResponse{}, fmt.Errorf("generate with eino agent: %w", context.Canceled)
	}}, policyRunnerTestPolicy(FallbackReturnError))
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	_, err = runner.Run(parent, policyRunnerTestRequest())
	if !errors.Is(err, ErrAgentCancelled) {
		t.Fatalf("Run() error = %v, want ErrAgentCancelled", err)
	}
}

func TestPolicyAwareRunnerFallsBackToLimitedAnswerWithoutTool(t *testing.T) {
	runner, err := NewPolicyAwareRunner(&fakePolicyRunner{run: func(_ context.Context, _ AgentRequest) (AgentResponse, error) {
		return AgentResponse{Answer: "模型自己的初步判断", ToolCalls: []ToolCall{{Name: "knowledge_search", Status: ToolCallFailed}}},
			errors.New("tool execution failed")
	}}, policyRunnerTestPolicy(FallbackAnswerWithoutTool))
	if err != nil {
		t.Fatal(err)
	}
	response, err := runner.Run(context.Background(), policyRunnerTestRequest())
	if err != nil {
		t.Fatalf("Run() error = %v, want fallback answer", err)
	}
	if response.Answer != "模型自己的初步判断" || len(response.Limitations) != 1 {
		t.Fatalf("response = %+v", response)
	}
}

func TestPolicyAwareRunnerKeepsErrorWhenFallbackAnswerIsEmpty(t *testing.T) {
	cause := errors.New("tool execution failed")
	runner, err := NewPolicyAwareRunner(&fakePolicyRunner{run: func(_ context.Context, _ AgentRequest) (AgentResponse, error) {
		return AgentResponse{}, cause
	}}, policyRunnerTestPolicy(FallbackAnswerWithoutTool))
	if err != nil {
		t.Fatal(err)
	}
	response, err := runner.Run(context.Background(), policyRunnerTestRequest())
	if !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v, want original cause", err)
	}
	if len(response.Limitations) != 1 || response.Limitations[0] != "agent execution failed" {
		t.Fatalf("limitations = %v", response.Limitations)
	}
}

func TestPolicyAwareRunnerRejectsInvalidConstruction(t *testing.T) {
	if _, err := NewPolicyAwareRunner(nil, policyRunnerTestPolicy(FallbackReturnError)); !errors.Is(err, ErrInvalidRunner) {
		t.Fatalf("NewPolicyAwareRunner(nil) error = %v", err)
	}
	invalid := policyRunnerTestPolicy(FallbackReturnError)
	invalid.Execution.Timeout = 0
	if _, err := NewPolicyAwareRunner(&fakePolicyRunner{}, invalid); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("invalid policy error = %v", err)
	}
	runner, err := NewPolicyAwareRunner(&fakePolicyRunner{}, policyRunnerTestPolicy(FallbackReturnError))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(context.Background(), AgentRequest{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid request error = %v", err)
	}
}
