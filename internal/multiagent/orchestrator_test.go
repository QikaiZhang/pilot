package multiagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"Pilot/internal/agent"
	"Pilot/internal/tools"
)

type fakeBranchTool struct {
	name  string
	calls atomic.Int32
	run   func(ctx context.Context, input json.RawMessage) (any, error)
}

func (t *fakeBranchTool) Name() string                { return t.name }
func (t *fakeBranchTool) Description() string         { return "fake branch tool" }
func (t *fakeBranchTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *fakeBranchTool) Execute(ctx context.Context, input json.RawMessage) (any, error) {
	t.calls.Add(1)
	return t.run(ctx, input)
}

var _ tools.Tool = (*fakeBranchTool)(nil)

func newTestOrchestrator(t *testing.T, model *fakeLLM, evidence, knowledge *fakeBranchTool) *Orchestrator {
	t.Helper()
	orchestrator, err := NewOrchestrator(model, evidence, knowledge, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return orchestrator
}

func healthTool() *fakeBranchTool {
	return &fakeBranchTool{name: "health_check", run: func(context.Context, json.RawMessage) (any, error) {
		return map[string]any{"healthy": true, "services": []string{"redis", "mysql", "es"}}, nil
	}}
}

func knowledgeTool() *fakeBranchTool {
	return &fakeBranchTool{name: "knowledge_search", run: func(_ context.Context, input json.RawMessage) (any, error) {
		return map[string]any{"query": string(input), "hit_count": 1}, nil
	}}
}

func testRequest() agent.AgentRequest {
	return agent.AgentRequest{UserID: "u1", SessionID: "s1", Query: "Redis 连接超时怎么排查"}
}

func TestOrchestratorIncidentRunsBothBranchesAndCites(t *testing.T) {
	model := &fakeLLM{responses: []fakeLLMResponse{
		{content: `{"intent":"incident_triage","confidence":0.9}`},
		{content: "健康检查显示依赖全部正常 [F1]，知识库建议检查连接池配置 [F2]。"},
	}}
	evidence, knowledge := healthTool(), knowledgeTool()
	orchestrator := newTestOrchestrator(t, model, evidence, knowledge)

	response, err := orchestrator.Run(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if evidence.calls.Load() != 1 || knowledge.calls.Load() != 1 {
		t.Fatalf("evidence calls = %d, knowledge calls = %d, want 1/1", evidence.calls.Load(), knowledge.calls.Load())
	}
	if response.Intent != string(IntentIncidentTriage) || len(response.Findings) != 2 {
		t.Fatalf("response = %+v", response)
	}
	if response.Findings[0].ID != "F1" || response.Findings[0].Role != string(RoleEvidence) {
		t.Fatalf("finding[0] = %+v, want F1/evidence", response.Findings[0])
	}
	if response.Findings[1].ID != "F2" || response.Findings[1].Role != string(RoleKnowledge) {
		t.Fatalf("finding[1] = %+v, want F2/knowledge", response.Findings[1])
	}
	if !strings.Contains(response.Answer, "[F1]") || len(response.Limitations) != 0 {
		t.Fatalf("answer = %q, limitations = %v", response.Answer, response.Limitations)
	}
	if response.Usage.InputTokens == 0 && response.Usage.OutputTokens == 0 {
		t.Fatalf("usage should accumulate classify + synthesis")
	}
}

func TestOrchestratorKnowledgeIntentSkipsEvidence(t *testing.T) {
	model := &fakeLLM{responses: []fakeLLMResponse{
		{content: `{"intent":"knowledge_query","confidence":0.9}`},
		{content: "知识库结论 [F1]。"},
	}}
	evidence := &fakeBranchTool{name: "health_check", run: func(context.Context, json.RawMessage) (any, error) {
		t.Fatal("evidence branch should not run for knowledge_query")
		return nil, nil
	}}
	knowledge := knowledgeTool()
	orchestrator := newTestOrchestrator(t, model, evidence, knowledge)

	response, err := orchestrator.Run(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(response.Findings) != 1 || response.Findings[0].ID != "F1" || response.Findings[0].Role != string(RoleKnowledge) {
		t.Fatalf("findings = %+v", response.Findings)
	}
}

func TestOrchestratorBranchFailureIsolation(t *testing.T) {
	model := &fakeLLM{responses: []fakeLLMResponse{
		{content: `{"intent":"incident_triage","confidence":0.9}`},
		{content: "知识库建议检查连接池 [F1]。"},
	}}
	evidence := &fakeBranchTool{name: "health_check", run: func(context.Context, json.RawMessage) (any, error) {
		return nil, errors.New("deps down")
	}}
	knowledge := knowledgeTool()
	orchestrator := newTestOrchestrator(t, model, evidence, knowledge)

	response, err := orchestrator.Run(context.Background(), testRequest())
	if err != nil {
		// 分支失败不中断整体：部分证据仍有价值。
		t.Fatalf("Run() error = %v, want degraded success", err)
	}
	if !containsString(response.Limitations, LimitationEvidenceBranch) {
		t.Fatalf("limitations = %v, want %s", response.Limitations, LimitationEvidenceBranch)
	}
	if len(response.Findings) != 1 || response.Findings[0].Role != string(RoleKnowledge) {
		t.Fatalf("findings = %+v", response.Findings)
	}
	if !strings.Contains(response.Answer, "[F1]") {
		t.Fatalf("answer = %q, want citation of remaining evidence", response.Answer)
	}
}

func TestOrchestratorCitationMissingFallsBackToDeterministic(t *testing.T) {
	model := &fakeLLM{responses: []fakeLLMResponse{
		{content: `{"intent":"incident_triage","confidence":0.9}`},
		{content: "凭经验看应该是网络问题，建议重启。"}, // 未引用任何证据 ID
	}}
	orchestrator := newTestOrchestrator(t, model, healthTool(), knowledgeTool())

	response, err := orchestrator.Run(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !containsString(response.Limitations, LimitationCitationMissing) {
		t.Fatalf("limitations = %v, want %s", response.Limitations, LimitationCitationMissing)
	}
	if !strings.Contains(response.Answer, "[F1]") {
		t.Fatalf("answer = %q, want deterministic evidence summary", response.Answer)
	}
}

func TestOrchestratorSynthesisFailureFallsBackToDeterministic(t *testing.T) {
	model := &fakeLLM{responses: []fakeLLMResponse{
		{content: `{"intent":"incident_triage","confidence":0.9}`},
		{err: errors.New("synthesis llm down")},
	}}
	orchestrator := newTestOrchestrator(t, model, healthTool(), knowledgeTool())

	response, err := orchestrator.Run(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Run() error = %v, want degraded success", err)
	}
	if !containsString(response.Limitations, LimitationSynthesisFailed) {
		t.Fatalf("limitations = %v, want %s", response.Limitations, LimitationSynthesisFailed)
	}
	if !strings.Contains(response.Answer, "[F1]") {
		t.Fatalf("answer = %q", response.Answer)
	}
}

func TestOrchestratorNoEvidenceSkipsSynthesis(t *testing.T) {
	model := &fakeLLM{responses: []fakeLLMResponse{
		{content: `{"intent":"incident_triage","confidence":0.9}`},
	}}
	brokenTool := func() *fakeBranchTool {
		return &fakeBranchTool{name: "broken", run: func(context.Context, json.RawMessage) (any, error) {
			return nil, errors.New("down")
		}}
	}
	orchestrator := newTestOrchestrator(t, model, brokenTool(), brokenTool())

	response, err := orchestrator.Run(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !containsString(response.Limitations, LimitationNoEvidence) ||
		!containsString(response.Limitations, LimitationEvidenceBranch) ||
		!containsString(response.Limitations, LimitationKnowledgeBranch) {
		t.Fatalf("limitations = %v", response.Limitations)
	}
	if len(model.requests) != 1 {
		// 只消耗了分诊调用：无证据时不应再花一次综合调用的钱。
		t.Fatalf("llm calls = %d, want 1 (classify only)", len(model.requests))
	}
	if response.Answer == "" {
		t.Fatal("answer should be deterministic no-evidence message")
	}
}

func TestOrchestratorInvalidRequest(t *testing.T) {
	orchestrator := newTestOrchestrator(t, &fakeLLM{}, healthTool(), knowledgeTool())
	if _, err := orchestrator.Run(context.Background(), agent.AgentRequest{}); !errors.Is(err, agent.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestOrchestratorRejectsMissingDependencies(t *testing.T) {
	if _, err := NewOrchestrator(nil, healthTool(), knowledgeTool(), nil, nil); !errors.Is(err, ErrOrchestratorDependency) {
		t.Fatalf("nil model error = %v", err)
	}
	if _, err := NewOrchestrator(&fakeLLM{}, nil, knowledgeTool(), nil, nil); !errors.Is(err, ErrOrchestratorDependency) {
		t.Fatalf("nil evidence error = %v", err)
	}
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
