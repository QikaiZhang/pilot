package multiagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	projectagent "Pilot/internal/agent"

	"github.com/redis/go-redis/v9"
)

func TestTaskIDForIsStableAndDistinct(t *testing.T) {
	base := projectagent.AgentRequest{UserID: "u1", SessionID: "s1", Query: "Redis 连接超时"}
	if taskIDFor(base) != taskIDFor(base) {
		t.Fatal("same session+query should derive the same task id")
	}
	otherQuery := base
	otherQuery.Query = "MySQL 主从延迟"
	if taskIDFor(base) == taskIDFor(otherQuery) {
		t.Fatal("different query should derive a different task id")
	}
	otherSession := base
	otherSession.SessionID = "s2"
	if taskIDFor(base) == taskIDFor(otherSession) {
		t.Fatal("different session should derive a different task id")
	}
}

func TestMemoryTaskStoreRoundTrip(t *testing.T) {
	store := NewMemoryTaskStore()
	state := TaskState{TaskID: "t1", Status: TaskStatusRunning, StartedAt: time.Now().UTC()}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, found, err := store.Load(context.Background(), "t1")
	if err != nil || !found {
		t.Fatalf("Load() = (%v, %v, %v)", loaded, found, err)
	}
	if loaded.TaskID != "t1" || loaded.Status != TaskStatusRunning {
		t.Fatalf("loaded = %+v", loaded)
	}
	if err := store.Delete(context.Background(), "t1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, found, _ := store.Load(context.Background(), "t1"); found {
		t.Fatal("snapshot should be gone after Delete")
	}
}

func TestRedisTaskStoreRejectsNilClient(t *testing.T) {
	if _, err := NewRedisTaskStore(nil); err == nil {
		t.Fatal("NewRedisTaskStore(nil) should fail")
	}
}

func TestRedisTaskStoreSurfacesConnectionErrors(t *testing.T) {
	// 与 memory 包的 Redis 测试同策略：指向必然拒绝连接的地址，只验证
	// 错误被包装上业务上下文，不验证网络语义。
	store, err := NewRedisTaskStore(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}))
	if err != nil {
		t.Fatalf("NewRedisTaskStore() error = %v", err)
	}
	ctx := context.Background()
	if err := store.Save(ctx, TaskState{TaskID: "t1"}); err == nil || !strings.Contains(err.Error(), "save task snapshot") {
		t.Fatalf("Save() error = %v, want wrapped save error", err)
	}
	if _, _, err := store.Load(ctx, "t1"); err == nil || !strings.Contains(err.Error(), "load task snapshot") {
		t.Fatalf("Load() error = %v, want wrapped load error", err)
	}
	if err := store.Delete(ctx, "t1"); err == nil || !strings.Contains(err.Error(), "delete task snapshot") {
		t.Fatalf("Delete() error = %v, want wrapped delete error", err)
	}
}

// resumeFixture 构造带快照存储的编排器和一次"中断现场"：
// complex 排障计划 + evidence 分支已完成的证据。
func resumeFixture(t *testing.T, store TaskStore, startedAt time.Time, status TaskStatus) (*Orchestrator, *fakeLLM, *fakeBranchTool, *fakeBranchTool) {
	t.Helper()
	model := &fakeLLM{responses: []fakeLLMResponse{
		{content: "运行证据显示依赖健康 [F1]，知识库建议检查连接池配置 [F2]。"},
	}}
	evidence := &fakeBranchTool{name: "health_check", run: func(context.Context, json.RawMessage) (any, error) {
		t.Fatal("evidence branch should be restored from snapshot, not re-executed")
		return nil, nil
	}}
	knowledge := &fakeBranchTool{name: "knowledge_search", run: func(context.Context, json.RawMessage) (any, error) {
		return map[string]any{"hit_count": 1}, nil
	}}
	orchestrator, err := NewOrchestrator(model, evidence, knowledge, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := PlanResult{
		Intent:     IntentIncidentTriage,
		Roles:      []Role{RoleEvidence, RoleKnowledge},
		Confidence: 0.9,
		Complexity: ComplexityComplex,
		Source:     PlanSourceLLM,
	}
	state := TaskState{
		TaskID:         taskIDFor(testRequest()),
		Query:          testRequest().Query,
		Status:         status,
		StartedAt:      startedAt,
		Plan:           &plan,
		CompletedRoles: []Role{RoleEvidence},
		Findings:       []projectagent.Finding{{Role: string(RoleEvidence), Source: "tool:health_check", Summary: "healthy"}},
	}
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	return orchestrator, model, evidence, knowledge
}

func TestOrchestratorResumesCompletedBranchesFromSnapshot(t *testing.T) {
	store := NewMemoryTaskStore()
	orchestrator, model, _, knowledge := resumeFixture(t, store, time.Now().UTC(), TaskStatusRunning)

	response, err := orchestrator.Run(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if knowledge.calls.Load() != 1 {
		t.Fatalf("knowledge calls = %d, want 1 (only the missing branch runs)", knowledge.calls.Load())
	}
	if len(response.Findings) != 2 || response.Findings[0].ID != "F1" || response.Findings[1].ID != "F2" {
		t.Fatalf("findings = %+v, want restored F1 + fresh F2 in role order", response.Findings)
	}
	if response.TaskID != taskIDFor(testRequest()) {
		t.Fatalf("task id = %q", response.TaskID)
	}
	if response.Complexity != string(ComplexityComplex) {
		t.Fatalf("complexity = %q, want complex", response.Complexity)
	}
	if !strings.Contains(response.Answer, "[F1]") || !strings.Contains(response.Answer, "[F2]") {
		t.Fatalf("answer = %q, want citations of both findings", response.Answer)
	}
	if len(model.requests) != 1 {
		// 计划来自快照，不再消耗分诊调用；只剩综合一次。
		t.Fatalf("llm calls = %d, want 1 (synthesis only)", len(model.requests))
	}
	// 终态主动删除快照：TTL 兜底之外的第二重保险。
	if _, found, _ := store.Load(context.Background(), taskIDFor(testRequest())); found {
		t.Fatal("snapshot should be deleted once the task reaches a terminal state")
	}
}

func TestOrchestratorDiscardsStaleSnapshot(t *testing.T) {
	store := NewMemoryTaskStore()
	// 快照超过续跑窗口：丢弃重跑，绝不基于过期证据作答。
	orchestrator, model, evidence, _ := resumeFixture(t, store, time.Now().Add(-2*DefaultSnapshotTTL), TaskStatusRunning)
	// 把"不该执行就 Fatal"的断言撤掉：过期快照的分支应当重新执行。
	evidence.run = func(context.Context, json.RawMessage) (any, error) {
		return map[string]any{"healthy": true}, nil
	}
	model.responses = []fakeLLMResponse{
		{content: `{"intent":"incident_triage","confidence":0.9,"complexity":"complex"}`},
		{content: "运行证据健康 [F1]，知识库建议检查连接池 [F2]。"},
	}

	response, err := orchestrator.Run(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if evidence.calls.Load() != 1 {
		t.Fatalf("evidence calls = %d, want 1 (stale snapshot must not be reused)", evidence.calls.Load())
	}
	if len(response.Findings) != 2 {
		t.Fatalf("findings = %+v, want fresh full run", response.Findings)
	}
}

func TestOrchestratorDiscardsTerminalSnapshot(t *testing.T) {
	store := NewMemoryTaskStore()
	orchestrator, model, evidence, _ := resumeFixture(t, store, time.Now().UTC(), TaskStatusSucceeded)
	evidence.run = func(context.Context, json.RawMessage) (any, error) {
		return map[string]any{"healthy": true}, nil
	}
	model.responses = []fakeLLMResponse{
		{content: `{"intent":"incident_triage","confidence":0.9,"complexity":"complex"}`},
		{content: "运行证据健康 [F1]，知识库建议检查连接池 [F2]。"},
	}

	if _, err := orchestrator.Run(context.Background(), testRequest()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if evidence.calls.Load() != 1 {
		t.Fatalf("evidence calls = %d, want 1 (terminal snapshot must not be resumed)", evidence.calls.Load())
	}
}

func TestOrchestratorSurvivesSnapshotStoreFailure(t *testing.T) {
	// 快照层是恢复优化，不是正确性依赖：存取全程失败时编排照常完成。
	store := &failingTaskStore{}
	model := &fakeLLM{responses: []fakeLLMResponse{
		{content: `{"intent":"incident_triage","confidence":0.9,"complexity":"complex"}`},
		{content: "健康检查正常 [F1]，知识库建议检查连接池 [F2]。"},
	}}
	orchestrator, err := NewOrchestrator(model, healthTool(), knowledgeTool(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := orchestrator.Run(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Run() error = %v, want success despite store failure", err)
	}
	if len(response.Findings) != 2 || !strings.Contains(response.Answer, "[F1]") {
		t.Fatalf("response = %+v", response)
	}
}

type failingTaskStore struct{}

func (failingTaskStore) Save(context.Context, TaskState) error { return errors.New("store down") }
func (failingTaskStore) Load(context.Context, string) (TaskState, bool, error) {
	return TaskState{}, false, errors.New("store down")
}
func (failingTaskStore) Delete(context.Context, string) error { return errors.New("store down") }
