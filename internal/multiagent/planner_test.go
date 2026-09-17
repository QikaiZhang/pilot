package multiagent

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"Pilot/internal/ai"
)

type fakeLLMResponse struct {
	content string
	err     error
	block   bool
}

// fakeLLM 是脚本化模型：按队列返回响应，可注入错误与阻塞（测超时回退）。
type fakeLLM struct {
	mu        sync.Mutex
	responses []fakeLLMResponse
	requests  []ai.ModelRequest
}

func (f *fakeLLM) Generate(ctx context.Context, request ai.ModelRequest) (ai.ModelResponse, error) {
	f.mu.Lock()
	f.requests = append(f.requests, request)
	if len(f.responses) == 0 {
		f.mu.Unlock()
		return ai.ModelResponse{}, errors.New("no scripted response")
	}
	next := f.responses[0]
	f.responses = f.responses[1:]
	f.mu.Unlock()
	if next.block {
		<-ctx.Done()
		return ai.ModelResponse{}, ctx.Err()
	}
	if next.err != nil {
		return ai.ModelResponse{}, next.err
	}
	// 固定的非零 usage：编排层断言"分类+综合的用量累加"需要可区分的值。
	return ai.ModelResponse{
		Message: ai.Message{Role: ai.RoleAssistant, Content: next.content},
		Usage:   ai.Usage{InputTokens: 7, OutputTokens: 3},
	}, nil
}

func (f *fakeLLM) Stream(context.Context, ai.ModelRequest) (ai.TokenStream, error) {
	return nil, errors.New("not used")
}

func TestRuleClassify(t *testing.T) {
	cases := []struct {
		query string
		want  Intent
	}{
		{"Redis 连接超时怎么排查", IntentIncidentTriage},
		{"订单服务 5xx 突然升高", IntentIncidentTriage},
		{"Redis 连接池为什么要设最大连接数", IntentIncidentTriage},
		{"什么是 RRF 融合", IntentKnowledgeQuery},
		{"怎么配置 Redis 连接池", IntentKnowledgeQuery},
		{"你好", IntentGeneral},
	}
	for _, tc := range cases {
		intent, confidence := ruleClassify(tc.query)
		if intent != tc.want {
			t.Fatalf("ruleClassify(%q) = %s, want %s", tc.query, intent, tc.want)
		}
		if confidence <= 0 || confidence > 1 {
			t.Fatalf("confidence = %v, want in (0,1]", confidence)
		}
	}
}

func TestHybridPlannerLLMOverridesRule(t *testing.T) {
	// 规则会判成 incident（含"超时"），LLM 高置信判成 knowledge_query：
	// LLM 结果应当生效。
	model := &fakeLLM{responses: []fakeLLMResponse{
		{content: `{"intent":"knowledge_query","confidence":0.9}`},
	}}
	planner := NewHybridPlanner(model, nil)
	plan := planner.Plan(context.Background(), "Redis 连接超时的原理是什么")
	if plan.Intent != IntentKnowledgeQuery || plan.Source != PlanSourceLLM {
		t.Fatalf("plan = %+v", plan)
	}
	if !slices.Equal(plan.Roles, []Role{RoleKnowledge}) {
		t.Fatalf("roles = %v, want [knowledge]", plan.Roles)
	}
}

func TestHybridPlannerFallsBackToRuleOnModelError(t *testing.T) {
	model := &fakeLLM{responses: []fakeLLMResponse{{err: errors.New("llm down")}}}
	planner := NewHybridPlanner(model, nil)
	plan := planner.Plan(context.Background(), "Redis 连接超时怎么排查")
	if plan.Intent != IntentIncidentTriage || plan.Source != PlanSourceRuleFallback {
		t.Fatalf("plan = %+v, want rule fallback", plan)
	}
	// 规则基线同样带复杂度分级：单服务症状 → simple，只跑知识分支。
	if !slices.Equal(plan.Roles, []Role{RoleKnowledge}) || plan.Complexity != ComplexitySimple {
		t.Fatalf("plan = %+v, want simple/knowledge-only", plan)
	}
	// 级联信号 → complex，双分支全跑。
	plan = planner.Plan(context.Background(), "集群大面积超时")
	if !slices.Equal(plan.Roles, []Role{RoleEvidence, RoleKnowledge}) || plan.Complexity != ComplexityComplex {
		t.Fatalf("plan = %+v, want complex/both", plan)
	}
}

func TestHybridPlannerFallsBackOnUnparseableOutput(t *testing.T) {
	model := &fakeLLM{responses: []fakeLLMResponse{{content: "我觉得这是个故障排查问题"}}}
	planner := NewHybridPlanner(model, nil)
	plan := planner.Plan(context.Background(), "Redis 连接超时怎么排查")
	if plan.Source != PlanSourceRuleFallback {
		t.Fatalf("source = %s, want rule fallback", plan.Source)
	}
}

func TestHybridPlannerWidensRolesOnLowConfidence(t *testing.T) {
	model := &fakeLLM{responses: []fakeLLMResponse{
		{content: `{"intent":"knowledge_query","confidence":0.3}`},
	}}
	planner := NewHybridPlanner(model, nil)
	plan := planner.Plan(context.Background(), "随便问问")
	if plan.Source != PlanSourceWidened {
		t.Fatalf("source = %s, want widened", plan.Source)
	}
	if !slices.Equal(plan.Roles, []Role{RoleEvidence, RoleKnowledge}) {
		t.Fatalf("roles = %v, want both branches", plan.Roles)
	}
	if plan.Complexity != ComplexityComplex {
		t.Fatalf("complexity = %s, want complex on widened path", plan.Complexity)
	}
}

func TestRuleComplexity(t *testing.T) {
	cases := []struct {
		query string
		want  Complexity
	}{
		{"订单集群大面积 5xx，多个服务连环超时", ComplexityComplex},
		{"缓存雪崩，所有服务都慢了", ComplexityComplex},
		{"订单服务接口变慢", ComplexitySimple},
		{"Redis 连接超时怎么排查", ComplexitySimple},
	}
	for _, tc := range cases {
		if got := ruleComplexity(tc.query); got != tc.want {
			t.Fatalf("ruleComplexity(%q) = %s, want %s", tc.query, got, tc.want)
		}
	}
}

func TestHybridPlannerSimpleIncidentRunsKnowledgeOnly(t *testing.T) {
	// 分级调度：简单故障只跑知识分支（低精度先行），省掉重分支开销。
	model := &fakeLLM{responses: []fakeLLMResponse{
		{content: `{"intent":"incident_triage","confidence":0.9,"complexity":"simple"}`},
	}}
	planner := NewHybridPlanner(model, nil)
	plan := planner.Plan(context.Background(), "订单服务接口变慢")
	if plan.Complexity != ComplexitySimple || !slices.Equal(plan.Roles, []Role{RoleKnowledge}) {
		t.Fatalf("plan = %+v, want simple/knowledge-only", plan)
	}
}

func TestHybridPlannerComplexIncidentRunsBothBranches(t *testing.T) {
	model := &fakeLLM{responses: []fakeLLMResponse{
		{content: `{"intent":"incident_triage","confidence":0.9,"complexity":"complex"}`},
	}}
	planner := NewHybridPlanner(model, nil)
	plan := planner.Plan(context.Background(), "集群大面积报错")
	if plan.Complexity != ComplexityComplex || !slices.Equal(plan.Roles, []Role{RoleEvidence, RoleKnowledge}) {
		t.Fatalf("plan = %+v, want complex/both", plan)
	}
}

func TestHybridPlannerClassifyTimeoutFallsBackToRule(t *testing.T) {
	model := &fakeLLM{responses: []fakeLLMResponse{{block: true}}}
	planner := NewHybridPlanner(model, nil)
	planner.classifyTimeout = 20 * time.Millisecond
	plan := planner.Plan(context.Background(), "订单服务挂了")
	if plan.Source != PlanSourceRuleFallback || plan.Intent != IntentIncidentTriage {
		t.Fatalf("plan = %+v, want rule fallback", plan)
	}
}

func TestParseIntentJSON(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    Intent
		wantCx  Complexity
		wantErr bool
	}{
		{"plain", `{"intent":"incident_triage","confidence":0.9,"complexity":"simple"}`, IntentIncidentTriage, ComplexitySimple, false},
		{"complexity defaults to complex", `{"intent":"incident_triage","confidence":0.9}`, IntentIncidentTriage, ComplexityComplex, false},
		{"invalid complexity treated as complex", `{"intent":"general","confidence":0.5,"complexity":"hack"}`, IntentGeneral, ComplexityComplex, false},
		{"with fences", "```json\n{\"intent\":\"general\",\"confidence\":0.5}\n```", IntentGeneral, ComplexityComplex, false},
		{"with think", "<think>分析一下</think>{\"intent\":\"knowledge_query\",\"confidence\":0.8}", IntentKnowledgeQuery, ComplexityComplex, false},
		{"confidence clamped", `{"intent":"general","confidence":7}`, IntentGeneral, ComplexityComplex, false},
		{"unknown intent", `{"intent":"hack","confidence":0.9}`, "", "", true},
		{"no json", "我不知道", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			intent, complexity, _, err := parseIntentJSON(tc.content)
			if tc.wantErr {
				if !errors.Is(err, ErrClassifyUnparseable) {
					t.Fatalf("error = %v, want ErrClassifyUnparseable", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseIntentJSON() error = %v", err)
			}
			if intent != tc.want || complexity != tc.wantCx {
				t.Fatalf("parseIntentJSON() = (%s, %s), want (%s, %s)", intent, complexity, tc.want, tc.wantCx)
			}
		})
	}
}
