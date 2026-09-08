package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	projectagent "Pilot/internal/agent"
	"Pilot/internal/chat"
)

// AgentHandler 提供带工具编排能力的同步 Agent 接口。
// 请求解析和 HTTP 状态码映射留在 Handler，业务顺序由 chat.AgentService 负责。
type AgentHandler struct {
	service *chat.AgentService
}

func NewAgentHandler(service *chat.AgentService) *AgentHandler {
	return &AgentHandler{service: service}
}

// Run 处理 POST /api/v1/agent/chat。
func (h *AgentHandler) Run(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		WriteError(w, http.StatusServiceUnavailable, "agent_unavailable", "agent service is unavailable")
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

	response, err := h.service.Run(r.Context(), request)
	if err != nil {
		if errors.Is(err, chat.ErrInvalidTurn) {
			WriteError(w, http.StatusBadRequest, "invalid_request", "user_id, session_id, and query are required")
			return
		}
		if errors.Is(err, chat.ErrAgentRunnerRequired) {
			WriteError(w, http.StatusServiceUnavailable, "agent_unavailable", "agent service is unavailable")
			return
		}
		if errors.Is(err, projectagent.ErrAgentTimeout) {
			// 超时语义优先于普通工具错误：客户端需要区分“失败了”和“还没算完”。
			WriteError(w, http.StatusGatewayTimeout, "agent_timeout", "agent execution timed out")
			return
		}
		if errors.Is(err, projectagent.ErrAgentCancelled) {
			// 客户端通常已断开；499 是非正式状态码，用于日志区分主动取消。
			WriteError(w, 499, "agent_cancelled", "agent execution was cancelled")
			return
		}
		WriteError(w, http.StatusInternalServerError, "agent_failed", "agent execution failed")
		return
	}
	writeJSON(w, http.StatusOK, response)
}
