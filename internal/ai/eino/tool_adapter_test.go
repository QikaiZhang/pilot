package eino

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	projecttools "Pilot/internal/tools"
)

type adapterTool struct {
	seenContext any
	input       json.RawMessage
	result      any
	err         error
}

type invalidSchemaTool struct{}

func (invalidSchemaTool) Name() string { return "invalid_tool" }

func (invalidSchemaTool) Description() string { return "invalid schema" }

func (invalidSchemaTool) Parameters() json.RawMessage { return json.RawMessage(`invalid`) }

func (invalidSchemaTool) Execute(context.Context, json.RawMessage) (any, error) { return nil, nil }

func (t *adapterTool) Name() string { return "test_tool" }

func (t *adapterTool) Description() string { return "test description" }

func (t *adapterTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`)
}

func (t *adapterTool) Execute(ctx context.Context, input json.RawMessage) (any, error) {
	t.seenContext = ctx.Value("request-id")
	t.input = append(json.RawMessage(nil), input...)
	return t.result, t.err
}

func TestNewToolAdapterInfo(t *testing.T) {
	adapter, err := NewToolAdapter(&adapterTool{})
	if err != nil {
		t.Fatal(err)
	}
	info, err := adapter.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "test_tool" || info.Desc != "test description" || info.ParamsOneOf == nil {
		t.Fatalf("info = %+v, want tool metadata and schema", info)
	}
}

func TestToolAdapterInvokableRun(t *testing.T) {
	projectTool := &adapterTool{result: map[string]string{"status": "up"}}
	adapter, err := NewToolAdapter(projectTool)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), "request-id", "req-1")
	output, err := adapter.InvokableRun(ctx, `{"query":"redis"}`)
	if err != nil {
		t.Fatal(err)
	}
	if output != `{"status":"up"}` {
		t.Fatalf("output = %s, want JSON result", output)
	}
	if string(projectTool.input) != `{"query":"redis"}` || projectTool.seenContext != "req-1" {
		t.Fatalf("tool execution = input %s, context %v", projectTool.input, projectTool.seenContext)
	}
}

func TestToolAdapterReturnsToolError(t *testing.T) {
	wantErr := errors.New("dependency unavailable")
	adapter, err := NewToolAdapter(&adapterTool{err: wantErr})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.InvokableRun(context.Background(), `{}`)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped tool error", err)
	}
}

func TestNewToolAdapterRejectsInvalidMetadata(t *testing.T) {
	_, err := NewToolAdapter(invalidSchemaTool{})
	if !errors.Is(err, ErrInvalidToolAdapter) {
		t.Fatalf("error = %v, want ErrInvalidToolAdapter", err)
	}
}

var _ projecttools.Tool = (*adapterTool)(nil)
