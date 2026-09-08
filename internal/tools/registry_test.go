package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type registryTestTool struct {
	name       string
	parameters json.RawMessage
}

func (t registryTestTool) Name() string { return t.name }

func (t registryTestTool) Description() string { return "test tool" }

func (t registryTestTool) Parameters() json.RawMessage { return t.parameters }

func (t registryTestTool) Execute(context.Context, json.RawMessage) (any, error) { return nil, nil }

func TestNewRegistryRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		tools []Tool
		want  error
	}{
		{name: "nil tool", tools: []Tool{nil}, want: ErrInvalidTool},
		{name: "empty name", tools: []Tool{registryTestTool{}}, want: ErrInvalidTool},
		{name: "duplicate name", tools: []Tool{registryTestTool{name: "health_check"}, registryTestTool{name: "health_check"}}, want: ErrDuplicateTool},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRegistry(tt.tools...)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRegistryListAllowedPreservesRegistrationOrder(t *testing.T) {
	registry, err := NewRegistry(
		registryTestTool{name: "health_check", parameters: json.RawMessage(`{"type":"object"}`)},
		registryTestTool{name: "knowledge_search", parameters: json.RawMessage(`{"type":"object"}`)},
	)
	if err != nil {
		t.Fatal(err)
	}

	tools, err := registry.ListAllowed([]string{"knowledge_search", "health_check"})
	if err != nil {
		t.Fatal(err)
	}
	if got := tools[0].Name(); got != "health_check" {
		t.Fatalf("first tool = %q, want registration order health_check", got)
	}
}

func TestRegistryEmptyAllowlistFailsClosed(t *testing.T) {
	registry, err := NewRegistry(registryTestTool{name: "health_check"})
	if err != nil {
		t.Fatal(err)
	}
	tools, err := registry.ListAllowed(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Fatalf("tools = %d, want 0", len(tools))
	}
}

func TestRegistryDefinitionsRejectsInvalidJSONSchema(t *testing.T) {
	registry, err := NewRegistry(registryTestTool{name: "broken", parameters: json.RawMessage(`not-json`)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Definitions([]string{"broken"})
	if !errors.Is(err, ErrInvalidTool) {
		t.Fatalf("error = %v, want ErrInvalidTool", err)
	}
}
