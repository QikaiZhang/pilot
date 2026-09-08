package eino

import (
	"context"
	"sort"
	"strings"
	"sync"

	projectagent "Pilot/internal/agent"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/flow/agent/react"
	callbacktemplate "github.com/cloudwego/eino/utils/callbacks"
)

type toolAuditRecorder struct {
	mu       sync.Mutex
	step     int
	sequence int
	records  []auditedToolCall
}

type auditedToolCall struct {
	sequence int
	call     projectagent.ToolCall
}

type auditCallKey struct{}

func newToolAuditRecorder() *toolAuditRecorder {
	return &toolAuditRecorder{}
}

func (r *toolAuditRecorder) callback() callbacks.Handler {
	modelHandler := &callbacktemplate.ModelCallbackHandler{OnStart: r.onModelStart}
	toolHandler := &callbacktemplate.ToolCallbackHandler{
		OnStart: r.onToolStart,
		OnEnd:   r.onToolEnd,
		OnError: r.onToolError,
	}
	return react.BuildAgentCallback(modelHandler, toolHandler)
}

func (r *toolAuditRecorder) onModelStart(ctx context.Context, _ *callbacks.RunInfo, _ *model.CallbackInput) context.Context {
	r.mu.Lock()
	r.step++
	step := r.step
	r.mu.Unlock()
	if state := policyRunStateFrom(ctx); state != nil {
		state.setStep(step)
	}
	return ctx
}

func (r *toolAuditRecorder) onToolStart(ctx context.Context, info *callbacks.RunInfo, input *tool.CallbackInput) context.Context {
	name := "unknown"
	if info != nil && strings.TrimSpace(info.Name) != "" {
		name = strings.TrimSpace(info.Name)
	}
	arguments := "{}"
	if input != nil && strings.TrimSpace(input.ArgumentsInJSON) != "" {
		arguments = input.ArgumentsInJSON
	}

	r.mu.Lock()
	r.sequence++
	entry := auditedToolCall{sequence: r.sequence, call: projectagent.ToolCall{
		Name: name, Arguments: []byte(arguments), Step: r.step, Status: projectagent.ToolCallFailed,
	}}
	r.records = append(r.records, entry)
	index := len(r.records) - 1
	r.mu.Unlock()
	return context.WithValue(ctx, auditCallKey{}, index)
}

func (r *toolAuditRecorder) onToolEnd(ctx context.Context, _ *callbacks.RunInfo, _ *tool.CallbackOutput) context.Context {
	r.update(ctx, projectagent.ToolCallSucceeded, "")
	return ctx
}

func (r *toolAuditRecorder) onToolError(ctx context.Context, _ *callbacks.RunInfo, _ error) context.Context {
	r.update(ctx, projectagent.ToolCallFailed, "tool execution failed")
	return ctx
}

func (r *toolAuditRecorder) update(ctx context.Context, status projectagent.ToolCallStatus, message string) {
	index, ok := ctx.Value(auditCallKey{}).(int)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if index < 0 || index >= len(r.records) {
		return
	}
	r.records[index].call.Status = status
	r.records[index].call.Error = message
}

func (r *toolAuditRecorder) snapshot() []projectagent.ToolCall {
	r.mu.Lock()
	records := append([]auditedToolCall(nil), r.records...)
	r.mu.Unlock()
	sort.Slice(records, func(i, j int) bool { return records[i].sequence < records[j].sequence })
	result := make([]projectagent.ToolCall, 0, len(records))
	for _, record := range records {
		result = append(result, record.call)
	}
	return result
}
