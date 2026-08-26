package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"Pilot/internal/ai"
	"Pilot/internal/memory"
)

type fakeMemoryStore struct {
	history []memory.ChatMessage
	loadErr error
	saveErr error
	saved   [][]memory.ChatMessage
	events  *[]string
}

func (f *fakeMemoryStore) LoadRecent(context.Context, string, string, int) ([]memory.ChatMessage, error) {
	if f.events != nil {
		*f.events = append(*f.events, "load")
	}
	return f.history, f.loadErr
}

func (f *fakeMemoryStore) SaveMessages(_ context.Context, messages []memory.ChatMessage) error {
	if f.events != nil {
		*f.events = append(*f.events, "save:"+string(messages[0].Role))
	}
	f.saved = append(f.saved, messages)
	return f.saveErr
}

type fakeChatModel struct {
	response ai.ModelResponse
	err      error
	requests []ai.ModelRequest
	events   *[]string
}

func (f *fakeChatModel) Generate(_ context.Context, request ai.ModelRequest) (ai.ModelResponse, error) {
	if f.events != nil {
		*f.events = append(*f.events, "generate")
	}
	f.requests = append(f.requests, request)
	return f.response, f.err
}

func (f *fakeChatModel) Stream(context.Context, ai.ModelRequest) (ai.TokenStream, error) {
	return nil, errors.New("not implemented")
}

func newTestService(t *testing.T, memoryStore *fakeMemoryStore, model *fakeChatModel) *Service {
	t.Helper()
	service, err := NewService(memoryStore, model, Config{
		HistoryLimit: 5,
		SystemPrompt: "你是运维排障助手。",
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ids := []string{"user-message", "assistant-message"}
	service.newID = func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	service.now = func() time.Time { return time.Date(2026, time.August, 23, 0, 0, 0, 0, time.UTC) }
	return service
}

func TestServiceGenerateBuildsContextAndSavesMessagesInOrder(t *testing.T) {
	events := []string{}
	memoryStore := &fakeMemoryStore{
		history: []memory.ChatMessage{{
			ID: "old-message", UserID: "user-1", SessionID: "session-1",
			Role: memory.RoleAssistant, Content: "请提供错误日志。",
		}},
		events: &events,
	}
	model := &fakeChatModel{
		response: ai.ModelResponse{
			Message: ai.Message{Role: ai.RoleAssistant, Content: "请检查 Redis 超时。"},
			Usage:   ai.Usage{InputTokens: 10, OutputTokens: 8},
		},
		events: &events,
	}

	response, err := newTestService(t, memoryStore, model).Generate(context.Background(), TurnRequest{
		UserID: "user-1", SessionID: "session-1", Query: "Redis 为什么超时？",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if response.Content != "请检查 Redis 超时。" || response.Usage.OutputTokens != 8 {
		t.Fatalf("Generate() = %+v, want model response", response)
	}

	wantEvents := []string{"load", "save:user", "generate", "save:assistant"}
	if len(events) != len(wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
	for i := range wantEvents {
		if events[i] != wantEvents[i] {
			t.Fatalf("events = %v, want %v", events, wantEvents)
		}
	}
	if len(model.requests) != 1 || len(model.requests[0].Messages) != 3 {
		t.Fatalf("model messages = %+v, want system, history, and user", model.requests)
	}
	if model.requests[0].Messages[2].Content != "Redis 为什么超时？" {
		t.Fatalf("last model message = %+v, want current user query", model.requests[0].Messages[2])
	}
	if len(memoryStore.saved) != 2 || memoryStore.saved[0][0].ID != "user-message" || memoryStore.saved[1][0].ID != "assistant-message" {
		t.Fatalf("saved messages = %+v", memoryStore.saved)
	}
}

func TestServiceGenerateKeepsUserMessageWhenModelFails(t *testing.T) {
	memoryStore := &fakeMemoryStore{}
	modelErr := errors.New("model unavailable")
	model := &fakeChatModel{err: modelErr}

	_, err := newTestService(t, memoryStore, model).Generate(context.Background(), TurnRequest{
		UserID: "user-1", SessionID: "session-1", Query: "检查告警",
	})
	if !errors.Is(err, modelErr) {
		t.Fatalf("Generate() error = %v, want wrapped %v", err, modelErr)
	}
	if len(memoryStore.saved) != 1 || memoryStore.saved[0][0].Role != memory.RoleUser {
		t.Fatalf("saved messages = %+v, want only the user message", memoryStore.saved)
	}
}

func TestServiceGenerateStopsWhenUserMessageCannotBeSaved(t *testing.T) {
	saveErr := errors.New("mysql unavailable")
	memoryStore := &fakeMemoryStore{saveErr: saveErr}
	model := &fakeChatModel{}

	_, err := newTestService(t, memoryStore, model).Generate(context.Background(), TurnRequest{
		UserID: "user-1", SessionID: "session-1", Query: "检查告警",
	})
	if !errors.Is(err, saveErr) {
		t.Fatalf("Generate() error = %v, want wrapped %v", err, saveErr)
	}
	if len(model.requests) != 0 {
		t.Fatalf("model Generate() calls = %d, want 0", len(model.requests))
	}
}
