package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"Pilot/internal/chat"
)

func TestWriteSSEEvent(t *testing.T) {
	recorder := httptest.NewRecorder()
	err := writeSSEEvent(recorder, chat.StreamEvent{
		Type:    chat.StreamEventContent,
		Content: "你好",
	})
	if err != nil {
		t.Fatalf("writeSSEEvent() error = %v", err)
	}

	if got := recorder.Body.String(); !strings.Contains(got, "data: {\"type\":\"content\",\"content\":\"你好\"}") || !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("body = %q, want one SSE data frame", got)
	}
}
