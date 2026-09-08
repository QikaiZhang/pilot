package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAgentRequestValidate(t *testing.T) {
	valid := AgentRequest{UserID: "u1", SessionID: "s1", Query: "检查 Redis"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	if err := (AgentRequest{UserID: "u1", SessionID: "s1"}).Validate(); err != ErrInvalidRequest {
		t.Fatalf("invalid request error = %v, want %v", err, ErrInvalidRequest)
	}
}

func TestAgentPolicyValidate(t *testing.T) {
	valid := AgentPolicy{
		Execution:    ExecutionPolicy{MaxSteps: 3, Timeout: time.Second},
		AllowedTools: []string{"knowledge_search"},
		Fallback:     FallbackPolicy{Mode: FallbackAnswerWithoutTool},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}

	invalid := valid
	invalid.Execution.MaxSteps = 0
	if err := invalid.Validate(); err != ErrInvalidPolicy {
		t.Fatalf("invalid policy error = %v, want %v", err, ErrInvalidPolicy)
	}
}

func TestAgentPolicyValidateRejectsInvalidAllowlist(t *testing.T) {
	base := AgentPolicy{Execution: ExecutionPolicy{MaxSteps: 3, Timeout: time.Second}, Fallback: FallbackPolicy{Mode: FallbackReturnError}}
	for _, allowed := range [][]string{{""}, {"health_check", "health_check"}} {
		policy := base
		policy.AllowedTools = allowed
		if !errors.Is(policy.Validate(), ErrInvalidPolicy) {
			t.Fatalf("allowed tools %v should be invalid", allowed)
		}
	}
}

func TestAgentPolicyAllowsTool(t *testing.T) {
	policy := AgentPolicy{AllowedTools: []string{" health_check "}}
	if !policy.AllowsTool("health_check") || policy.AllowsTool("knowledge_search") || policy.AllowsTool("") {
		t.Fatal("unexpected tool allowlist result")
	}
}

func TestAgentPolicyContextKeepsParentCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	ctx, cancel := (AgentPolicy{Execution: ExecutionPolicy{Timeout: time.Minute}}).Context(parent)
	defer cancel()
	cancelParent()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("derived context did not observe parent cancellation")
	}
}
