package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"Pilot/internal/chat"
)

// Stream 处理 POST /api/v1/chat/stream。
// 业务编排与三条生命周期顺序（LoadRecent -> 保存 user -> 启动模型流）
// 都留在 chat.Service.Stream；控制层只负责把 EventStream 编码成 SSE 帧。
func (h *ChatHandler) Stream(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		WriteError(w, http.StatusInternalServerError, "chat_unavailable", "chat service is unavailable")
		return
	}

	var request chat.TurnRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxChatRequestBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		WriteError(w, http.StatusBadRequest, "invalid_request", "request body must contain one JSON object")
		return
	}

	setSSEHeaders(w)
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "stream_unavailable", "streaming is not supported")
		return
	}

	stream, err := h.service.Stream(r.Context(), request)
	if err != nil {
		if errors.Is(err, chat.ErrInvalidTurn) {
			WriteError(w, http.StatusBadRequest, "invalid_request", "user_id, session_id, and query are required")
			return
		}
		WriteError(w, http.StatusInternalServerError, "chat_failed", "chat stream failed to start")
		return
	}
	// 无论正常结束还是 panic，都释放底层 TokenStream。
	defer stream.Close()

	// WARNING S级核心逻辑：理解后必须删除本区域，并从零独立重写，禁止直接复用。
	// 删除范围：从 BEGIN S_LEVEL_REFERENCE 到 END S_LEVEL_REFERENCE。
	// BEGIN S_LEVEL_REFERENCE
	for {
		event, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			return
		}
		if err := writeSSEEvent(w, event); err != nil {
			return
		}
		flusher.Flush()
	}
	// END S_LEVEL_REFERENCE
}
