package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Pilot/internal/ai"
	"Pilot/internal/chat"
	"Pilot/internal/memory"
)

type streamModelStub struct {
	stream ai.TokenStream
	err    error
}

func (m streamModelStub) Generate(context.Context, ai.ModelRequest) (ai.ModelResponse, error) {
	return ai.ModelResponse{}, errors.New("generate is not used in stream test")
}

func (m streamModelStub) Stream(context.Context, ai.ModelRequest) (ai.TokenStream, error) {
	return m.stream, m.err
}

type streamMemoryStub struct{}

func (streamMemoryStub) LoadRecent(context.Context, string, string, int) ([]memory.ChatMessage, error) {
	return nil, nil
}

func (streamMemoryStub) SaveMessages(context.Context, []memory.ChatMessage) error {
	return nil
}

// flushRecorder wraps httptest.ResponseRecorder so it satisfies http.Flusher.
type flushRecorder struct {
	*httptest.ResponseRecorder
}

func (f flushRecorder) Flush() {}

func newTestStreamHandler(t *testing.T, model ai.ChatModel) *Chat {
	t.Helper()
	service, err := chat.NewService(streamMemoryStub{}, model, chat.Config{
		HistoryLimit: 4,
		SystemPrompt: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewChat(service)
}

type oneShotStream struct {
	sent     bool
	closed   int
	closeErr error
}

func (o *oneShotStream) Recv() (ai.ModelChunk, error) {
	if !o.sent {
		o.sent = true
		return ai.ModelChunk{Delta: "hi"}, nil
	}
	return ai.ModelChunk{}, io.EOF
}

func (o *oneShotStream) Close() error {
	o.closed++
	return o.closeErr
}

func TestChatStreamWritesFramesAndFinishesAtEOF(t *testing.T) {
	upstream := &oneShotStream{}
	handler := newTestStreamHandler(t, streamModelStub{stream: upstream})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/chat/stream", strings.NewReader(`{"user_id":"u1","session_id":"s1","query":"hello"}`))

	handler.Stream(flushRecorder{recorder}, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"type":"content"`) || !strings.Contains(body, `"type":"done"`) {
		t.Fatalf("body = %q, want content and done frames", body)
	}
	if !strings.Contains(body, "\n\n") {
		t.Fatalf("body = %q, want SSE frame separators", body)
	}
	if upstream.closed != 1 {
		t.Fatalf("upstream Close() calls = %d, want 1", upstream.closed)
	}
}
