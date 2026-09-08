package eino

import (
	"context"
	"encoding/json"
	"testing"

	projectagent "Pilot/internal/agent"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/tool"
)

func TestToolAuditRecorderRecordsSuccessAndStep(t *testing.T) {
	recorder := newToolAuditRecorder()
	recorder.step = 2
	ctx := recorder.onToolStart(context.Background(), &callbacks.RunInfo{Name: "health_check"}, &tool.CallbackInput{ArgumentsInJSON: `{"scope":"all"}`})
	recorder.onToolEnd(ctx, nil, &tool.CallbackOutput{Response: `{"healthy":true}`})

	records := recorder.snapshot()
	if len(records) != 1 {
		t.Fatalf("records = %+v", records)
	}
	if records[0].Name != "health_check" || records[0].Step != 2 || records[0].Status != projectagent.ToolCallSucceeded {
		t.Fatalf("record = %+v", records[0])
	}
	if !json.Valid(records[0].Arguments) {
		t.Fatalf("arguments = %s, want valid JSON", records[0].Arguments)
	}
}

func TestToolAuditRecorderSanitizesErrors(t *testing.T) {
	recorder := newToolAuditRecorder()
	ctx := recorder.onToolStart(context.Background(), nil, nil)
	recorder.onToolError(ctx, nil, context.DeadlineExceeded)

	records := recorder.snapshot()
	if len(records) != 1 || records[0].Status != projectagent.ToolCallFailed {
		t.Fatalf("records = %+v", records)
	}
	if records[0].Error != "tool execution failed" {
		t.Fatalf("error = %q, want sanitized summary", records[0].Error)
	}
}
