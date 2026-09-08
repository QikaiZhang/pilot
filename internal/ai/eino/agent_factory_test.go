package eino

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	projectagent "Pilot/internal/agent"
	projecttools "Pilot/internal/tools"
)

type factoryTestTool struct {
	name string
}

func (t *factoryTestTool) Name() string { return t.name }

func (t *factoryTestTool) Description() string { return "factory test tool" }

func (t *factoryTestTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (*factoryTestTool) Execute(context.Context, json.RawMessage) (any, error) { return nil, nil }

func TestNewReActAgentRequiresDependencies(t *testing.T) {
	policy := projectagent.AgentPolicy{
		Execution:    projectagent.ExecutionPolicy{MaxSteps: 2, Timeout: time.Second},
		Fallback:     projectagent.FallbackPolicy{Mode: projectagent.FallbackReturnError},
		AllowedTools: []string{"health_check"},
	}
	registry, err := projecttools.NewRegistry(projecttools.NewHealthCheck(nil))
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewReActAgent(t.Context(), nil, registry, policy)
	if !errors.Is(err, ErrToolCallingModelRequired) {
		t.Fatalf("error = %v, want ErrToolCallingModelRequired", err)
	}
}

func TestNewEinoToolsUsesPolicyAllowlist(t *testing.T) {
	registry, err := projecttools.NewRegistry(
		projecttools.NewHealthCheck(nil),
		&factoryTestTool{name: "knowledge_search"},
	)
	if err != nil {
		t.Fatal(err)
	}
	policy := projectagent.AgentPolicy{AllowedTools: []string{"health_check"}}

	einoTools, err := newEinoTools(registry, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(einoTools) != 1 {
		t.Fatalf("tools = %d, want 1", len(einoTools))
	}
	info, err := einoTools[0].Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "health_check" {
		t.Fatalf("tool name = %q, want health_check", info.Name)
	}
}

func TestNewEinoToolsRejectsUnknownAllowedTool(t *testing.T) {
	registry, err := projecttools.NewRegistry(projecttools.NewHealthCheck(nil))
	if err != nil {
		t.Fatal(err)
	}
	_, err = newEinoTools(registry, projectagent.AgentPolicy{AllowedTools: []string{"missing"}})
	if !errors.Is(err, projecttools.ErrToolNotFound) {
		t.Fatalf("error = %v, want ErrToolNotFound", err)
	}
}
