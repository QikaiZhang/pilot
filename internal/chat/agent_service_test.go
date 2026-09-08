package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	projectagent "Pilot/internal/agent"
	"Pilot/internal/ai"
	"Pilot/internal/memory"
)

type fakeAgentRunner struct {
	response projectagent.AgentResponse
	err      error
	request  projectagent.AgentRequest
	events   *[]string
}

func (f *fakeAgentRunner) Run(_ context.Context, request projectagent.AgentRequest) (projectagent.AgentResponse, error) {
	f.request = request
	if f.events != nil {
		*f.events = append(*f.events, "run")
	}
	return f.response, f.err
}

func newTestAgentService(t *testing.T, store *fakeMemoryStore, runner *fakeAgentRunner) *AgentService {
	t.Helper()
	service, err := NewAgentService(store, runner, Config{HistoryLimit: 5, SystemPrompt: "你是 Agent 助手。"})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{"user-message", "assistant-message"}
	service.newID = func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	service.now = func() time.Time { return time.Date(2026, time.August, 31, 0, 0, 0, 0, time.UTC) }
	return service
}

func TestAgentServiceRunBuildsHistoryAndPersistsFinalAnswer(t *testing.T) {
	events := []string{}
	store := &fakeMemoryStore{
		history: []memory.ChatMessage{{Role: memory.RoleAssistant, Content: "之前的回答"}},
		events:  &events,
	}
	runner := &fakeAgentRunner{
		response: projectagent.AgentResponse{Answer: "  最终答案  ", Usage: ai.Usage{OutputTokens: 12}},
		events:   &events,
	}

	response, err := newTestAgentService(t, store, runner).Run(context.Background(), TurnRequest{UserID: "u1", SessionID: "s1", Query: "检查服务"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if response.Answer != "最终答案" || response.Usage.OutputTokens != 12 {
		t.Fatalf("response = %+v", response)
	}
	wantEvents := []string{"load", "save:user", "run", "save:assistant"}
	if len(events) != len(wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
	for i := range wantEvents {
		if events[i] != wantEvents[i] {
			t.Fatalf("events = %v, want %v", events, wantEvents)
		}
	}
	if len(runner.request.History) != 2 || runner.request.History[0].Role != ai.RoleSystem || runner.request.History[1].Role != ai.RoleAssistant {
		t.Fatalf("history = %+v, want system and stored history", runner.request.History)
	}
	if len(store.saved) != 2 || store.saved[1][0].Content != "  最终答案  " {
		t.Fatalf("saved = %+v", store.saved)
	}
}

func TestAgentServiceRunKeepsUserMessageWhenRunnerFails(t *testing.T) {
	store := &fakeMemoryStore{}
	runnerErr := errors.New("model unavailable")
	runner := &fakeAgentRunner{err: runnerErr}

	_, err := newTestAgentService(t, store, runner).Run(context.Background(), TurnRequest{UserID: "u1", SessionID: "s1", Query: "检查服务"})
	if !errors.Is(err, runnerErr) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, runnerErr)
	}
	if len(store.saved) != 1 || store.saved[0][0].Role != memory.RoleUser {
		t.Fatalf("saved = %+v, want only user message", store.saved)
	}
}

func TestNewAgentServiceRequiresRunner(t *testing.T) {
	_, err := NewAgentService(&fakeMemoryStore{}, nil, Config{HistoryLimit: 1})
	if !errors.Is(err, ErrAgentRunnerRequired) {
		t.Fatalf("NewAgentService() error = %v", err)
	}
}

func TestAgentServiceRunRejectsEmptyAnswerWithoutSavingAssistant(t *testing.T) {
	store := &fakeMemoryStore{}
	runner := &fakeAgentRunner{response: projectagent.AgentResponse{Answer: "   "}}

	_, err := newTestAgentService(t, store, runner).Run(context.Background(), TurnRequest{UserID: "u1", SessionID: "s1", Query: "检查服务"})
	if !errors.Is(err, ErrAgentAnswerEmpty) {
		t.Fatalf("Run() error = %v, want %v", err, ErrAgentAnswerEmpty)
	}
	if len(store.saved) != 1 || store.saved[0][0].Role != memory.RoleUser {
		t.Fatalf("saved = %+v, want only user message", store.saved)
	}
}
