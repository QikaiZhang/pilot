package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"Pilot/internal/chat"
)

// writeSSEEvent 将聊天层事件编码成一条完整 SSE data 帧。
// Flush 由调用方控制，因为调用方还需要决定何时结束连接。
func writeSSEEvent(w http.ResponseWriter, event chat.StreamEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal sse event: %w", err)
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return fmt.Errorf("write sse event: %w", err)
	}
	return nil
}

func setSSEHeaders(w http.ResponseWriter) {
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
}
