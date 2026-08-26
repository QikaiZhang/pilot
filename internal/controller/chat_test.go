package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Pilot/internal/ai"
	"Pilot/internal/chat"
	"Pilot/internal/memory"
)

type chatMemoryStub struct{}

func (chatMemoryStub) LoadRecent(context.Context, string, string, int) ([]memory.ChatMessage, error) {
	return nil, nil
}

func (chatMemoryStub) SaveMessages(context.Context, []memory.ChatMessage) error {
	return nil
}

type chatModelStub struct{}

func (chatModelStub) Generate(context.Context, ai.ModelRequest) (ai.ModelResponse, error) {
	return ai.ModelResponse{Message: ai.Message{Role: ai.RoleAssistant, Content: "ok"}}, nil
}

func (chatModelStub) Stream(context.Context, ai.ModelRequest) (ai.TokenStream, error) {
	return nil, errors.New("stream is not used in this test")
}

func newTestChatHandler(t *testing.T) *Chat {
	t.Helper()
	service, err := chat.NewService(chatMemoryStub{}, chatModelStub{}, chat.Config{
		HistoryLimit: 4,
		SystemPrompt: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewChat(service)
}

func TestChatGenerate(t *testing.T) {
	handler := newTestChatHandler(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/chat", strings.NewReader(`{"user_id":"u1","session_id":"s1","query":"hello"}`))

	handler.Generate(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), `"content":"ok"`) {
		t.Fatalf("body = %s, want assistant content", recorder.Body.String())
	}
}

func TestChatGenerateRejectsInvalidRequest(t *testing.T) {
	handler := newTestChatHandler(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/chat", strings.NewReader(`{"user_id":"u1","session_id":"s1"}`))

	handler.Generate(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
