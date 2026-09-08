package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"Pilot/internal/chat"
)

const maxChatRequestBody = 1 << 20

// ChatHandler 提供同步对话 HTTP 接口。
// 业务编排留在 chat.Service，控制层只负责协议转换和状态码映射。
type ChatHandler struct {
	service *chat.Service
}

func NewChatHandler(service *chat.Service) *ChatHandler {
	return &ChatHandler{service: service}
}

// Generate 处理 POST /api/v1/chat。
func (h *ChatHandler) Generate(w http.ResponseWriter, r *http.Request) {
	//防御性检查
	if h == nil || h.service == nil {
		WriteError(w, http.StatusInternalServerError, "chat_unavailable", "chat service is unavailable")
		return
	}

	//请求体解析
	var request chat.TurnRequest
	//防止大 json 攻击
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxChatRequestBody))
	decoder.DisallowUnknownFields()
	//强制对齐实现严格模式的 api
	if err := decoder.Decode(&request); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	var extra any
	//防止 json走私攻击，这里只处理一个 json 的情况
	if err := decoder.Decode(&extra); err != io.EOF {
		WriteError(w, http.StatusBadRequest, "invalid_request", "request body must contain one JSON object")
		return
	}
	//业务调用
	response, err := h.service.Generate(r.Context(), request)
	//错误的映射
	if err != nil {
		if errors.Is(err, chat.ErrInvalidTurn) {
			WriteError(w, http.StatusBadRequest, "invalid_request", "user_id, session_id, and query are required")
			return
		}
		WriteError(w, http.StatusInternalServerError, "chat_failed", "chat generation failed")
		return
	}
	writeJSON(w, http.StatusOK, response)
}
