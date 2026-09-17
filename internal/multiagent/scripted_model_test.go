package multiagent

import (
	"context"
	"strings"
	"testing"
	"time"

	projectagent "Pilot/internal/agent"
	"Pilot/internal/ai"
)

func TestScriptedModelClassifyReturnsParsableIntent(t *testing.T) {
	model := NewScriptedModel()
	response, err := model.Generate(context.Background(), ai.ModelRequest{
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: classifySystemPrompt},
			{Role: ai.RoleUser, Content: "Redis 集群大面积超时"},
		},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	intent, complexity, confidence, err := parseIntentJSON(response.Message.Content)
	if err != nil {
		t.Fatalf("scripted classification is unparseable: %v", err)
	}
	if intent != IntentIncidentTriage || complexity != ComplexityComplex || confidence <= 0 {
		t.Fatalf("classification = (%s, %s, %v)", intent, complexity, confidence)
	}
}

func TestScriptedModelSynthesisCitesAllEvidence(t *testing.T) {
	model := NewScriptedModel()
	findings := []projectagent.Finding{
		{ID: "F1", Role: "evidence", Source: "tool:health_check", Summary: "healthy"},
		{ID: "F2", Role: "knowledge", Source: "tool:knowledge_search", Summary: "check pool"},
	}
	response, err := model.Generate(context.Background(), ai.ModelRequest{
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: synthesisSystemPrompt},
			{Role: ai.RoleUser, Content: buildSynthesisPrompt("Redis 超时", findings, defaultFindingRunes)},
		},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	answer := response.Message.Content
	if !strings.Contains(answer, "[F1]") || !strings.Contains(answer, "[F2]") {
		t.Fatalf("answer = %q, want citations of both evidence ids", answer)
	}
}

func TestScriptedModelSynthesisOverrideDrivesCitationFallback(t *testing.T) {
	// 微基准的 no_citation 场景注入口：覆盖回复后编排器必须走引用缺失降级。
	model := ScriptedModel{SynthesisAnswer: "凭经验看是网络抖动，建议重启网关。"}
	response, err := model.Generate(context.Background(), ai.ModelRequest{
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: synthesisSystemPrompt},
			{Role: ai.RoleUser, Content: "证据 [F1]：healthy"},
		},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if strings.Contains(response.Message.Content, "[F1]") {
		t.Fatalf("override answer = %q, want no citation", response.Message.Content)
	}
}

func TestScriptedModelClassifyDelayRespectsContext(t *testing.T) {
	model := ScriptedModel{ClassifyDelay: 50 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if _, err := model.Generate(ctx, ai.ModelRequest{
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: classifySystemPrompt},
			{Role: ai.RoleUser, Content: "Redis 超时"},
		},
	}); err == nil {
		t.Fatal("Generate() should return the context error when the delay exceeds the deadline")
	}
}
