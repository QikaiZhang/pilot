package eino

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	projectagent "Pilot/internal/agent"
	"github.com/cloudwego/eino/compose"
)

func TestPolicyMiddlewareSoftRejectsOversizedArguments(t *testing.T) {
	state := &policyRunState{}
	called := false
	next := func(context.Context, *compose.ToolInput) (*compose.ToolOutput, error) {
		called = true
		return &compose.ToolOutput{Result: `{"ok":true}`}, nil
	}
	middleware := newPolicyMiddleware(projectagent.AgentPolicy{
		Budget: projectagent.BudgetPolicy{MaxArgumentsBytes: 4},
	})

	output, err := middleware.Invokable(next)(withPolicyRunState(context.Background(), state), &compose.ToolInput{
		Name:      "health_check",
		CallID:    "call-1",
		Arguments: `{"too":"large"}`,
	})
	if err != nil {
		t.Fatalf("middleware error = %v, want nil soft rejection", err)
	}
	if called {
		t.Fatal("next was called for oversized arguments")
	}
	var result struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(output.Result), &result); err != nil {
		t.Fatalf("rejection result is invalid JSON: %v", err)
	}
	if result.OK || result.Error.Code != policyArgumentsTooLargeCode {
		t.Fatalf("rejection result = %s", output.Result)
	}
	rejected := state.rejectedSnapshot()
	if len(rejected) != 1 || rejected[0].Status != projectagent.ToolCallRejected || rejected[0].Name != "health_check" {
		t.Fatalf("rejected audit = %+v", rejected)
	}
}

func TestPolicyMiddlewareAllowsArgumentsWithinLimit(t *testing.T) {
	called := false
	next := func(context.Context, *compose.ToolInput) (*compose.ToolOutput, error) {
		called = true
		return &compose.ToolOutput{Result: `{"ok":true}`}, nil
	}
	middleware := newPolicyMiddleware(projectagent.AgentPolicy{
		Budget: projectagent.BudgetPolicy{MaxArgumentsBytes: 20},
	})
	if _, err := middleware.Invokable(next)(context.Background(), &compose.ToolInput{
		Name:      "health_check",
		Arguments: `{}`,
	}); err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if !called {
		t.Fatal("next was not called for arguments within limit")
	}
}

func TestPolicyMiddlewareConcurrentRejectionsProtectRunState(t *testing.T) {
	state := &policyRunState{}
	next := func(context.Context, *compose.ToolInput) (*compose.ToolOutput, error) {
		t.Fatal("next was called for oversized arguments")
		return nil, nil
	}
	middleware := newPolicyMiddleware(projectagent.AgentPolicy{
		Budget: projectagent.BudgetPolicy{MaxArgumentsBytes: 1},
	})
	invoke := middleware.Invokable(next)

	const calls = 32
	var group sync.WaitGroup
	group.Add(calls)
	for i := 0; i < calls; i++ {
		go func() {
			defer group.Done()
			_, _ = invoke(withPolicyRunState(context.Background(), state), &compose.ToolInput{
				Name:      "health_check",
				Arguments: `{}`,
			})
		}()
	}
	group.Wait()
	if got := len(state.rejectedSnapshot()); got != calls {
		t.Fatalf("rejected audit count = %d, want %d", got, calls)
	}
}

func TestPolicyMiddlewareRejectsCallsAfterBudgetIsReserved(t *testing.T) {
	state := &policyRunState{}
	called := 0
	next := func(context.Context, *compose.ToolInput) (*compose.ToolOutput, error) {
		called++
		return &compose.ToolOutput{Result: `{"ok":true}`}, nil
	}
	middleware := newPolicyMiddleware(projectagent.AgentPolicy{
		Budget: projectagent.BudgetPolicy{MaxToolCalls: 2, MaxArgumentsBytes: 100},
	})
	invoke := middleware.Invokable(next)
	ctx := withPolicyRunState(context.Background(), state)
	for i := 0; i < 3; i++ {
		output, err := invoke(ctx, &compose.ToolInput{Name: "health_check", Arguments: `{}`})
		if err != nil {
			t.Fatalf("call %d error = %v", i, err)
		}
		if i == 2 {
			var result struct {
				OK    bool `json:"ok"`
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(output.Result), &result); err != nil {
				t.Fatalf("budget rejection is invalid JSON: %v", err)
			}
			if result.OK || result.Error.Code != policyToolCallsExceededCode {
				t.Fatalf("budget rejection = %s", output.Result)
			}
		}
	}
	if called != 2 {
		t.Fatalf("next calls = %d, want 2", called)
	}
	if got := len(state.rejectedSnapshot()); got != 1 {
		t.Fatalf("rejected audit count = %d, want 1", got)
	}
}

func TestPolicyMiddlewareConcurrentBudgetNeverExceedsLimit(t *testing.T) {
	state := &policyRunState{}
	const limit = 8
	var mu sync.Mutex
	called := 0
	next := func(context.Context, *compose.ToolInput) (*compose.ToolOutput, error) {
		mu.Lock()
		called++
		mu.Unlock()
		return &compose.ToolOutput{Result: `{"ok":true}`}, nil
	}
	middleware := newPolicyMiddleware(projectagent.AgentPolicy{
		Budget: projectagent.BudgetPolicy{MaxToolCalls: limit, MaxArgumentsBytes: 100},
	})
	invoke := middleware.Invokable(next)
	ctx := withPolicyRunState(context.Background(), state)
	const attempts = 64
	var group sync.WaitGroup
	group.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer group.Done()
			_, _ = invoke(ctx, &compose.ToolInput{Name: "health_check", Arguments: `{}`})
		}()
	}
	group.Wait()
	if called != limit {
		t.Fatalf("next calls = %d, want %d", called, limit)
	}
}

func TestPolicyMiddlewareSoftRejectsRepeatedFailedCall(t *testing.T) {
	state := &policyRunState{}
	called := 0
	next := func(context.Context, *compose.ToolInput) (*compose.ToolOutput, error) {
		called++
		return nil, errors.New("dependency unavailable")
	}
	middleware := newPolicyMiddleware(projectagent.AgentPolicy{
		Budget: projectagent.BudgetPolicy{MaxToolCalls: 10, MaxArgumentsBytes: 100},
		Repeat: projectagent.RepeatCallPolicy{MaxConsecutiveFailures: 2},
	})
	invoke := middleware.Invokable(next)
	ctx := withPolicyRunState(context.Background(), state)
	input := &compose.ToolInput{Name: "knowledge_search", Arguments: `{"query":"redis"}`}

	for i := 0; i < 2; i++ {
		if _, err := invoke(ctx, input); err == nil {
			t.Fatalf("failure call %d returned nil error", i+1)
		}
	}
	output, err := invoke(ctx, input)
	if err != nil {
		t.Fatalf("repeated call error = %v, want soft rejection", err)
	}
	if called != 2 {
		t.Fatalf("next calls = %d, want 2", called)
	}
	var result struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(output.Result), &result); err != nil {
		t.Fatalf("rejection result is invalid JSON: %v", err)
	}
	if result.OK || result.Error.Code != policyRepeatedFailureCode {
		t.Fatalf("rejection result = %s", output.Result)
	}
	rejected := state.rejectedSnapshot()
	if len(rejected) != 1 || rejected[0].Status != projectagent.ToolCallRejected {
		t.Fatalf("rejected audit = %+v", rejected)
	}
}

func TestPolicyMiddlewareSuccessfulCallResetsFailureCount(t *testing.T) {
	state := &policyRunState{}
	called := 0
	next := func(context.Context, *compose.ToolInput) (*compose.ToolOutput, error) {
		called++
		if called == 1 {
			return nil, errors.New("temporary failure")
		}
		return &compose.ToolOutput{Result: `{"ok":true}`}, nil
	}
	middleware := newPolicyMiddleware(projectagent.AgentPolicy{
		Budget: projectagent.BudgetPolicy{MaxToolCalls: 10, MaxArgumentsBytes: 100},
		Repeat: projectagent.RepeatCallPolicy{MaxConsecutiveFailures: 2},
	})
	invoke := middleware.Invokable(next)
	ctx := withPolicyRunState(context.Background(), state)
	input := &compose.ToolInput{Name: "health_check", Arguments: `{}`}
	if _, err := invoke(ctx, input); err == nil {
		t.Fatal("first call should fail")
	}
	if _, err := invoke(ctx, input); err != nil {
		t.Fatalf("second call should succeed: %v", err)
	}
	if _, err := invoke(ctx, input); err != nil {
		t.Fatalf("third call should remain available after reset: %v", err)
	}
	if called != 3 {
		t.Fatalf("next calls = %d, want 3", called)
	}
}

func TestToolCallFingerprintCanonicalizesObjectKeyOrder(t *testing.T) {
	first := toolCallFingerprint("knowledge_search", `{"query":"redis","top_k":3}`)
	second := toolCallFingerprint("knowledge_search", `{"top_k":3,"query":"redis"}`)
	if first != second {
		t.Fatalf("fingerprints differ for equivalent objects: %q vs %q", first, second)
	}
	if first == toolCallFingerprint("knowledge_search", `{"query":"mysql","top_k":3}`) {
		t.Fatal("different arguments produced the same fingerprint")
	}
}
