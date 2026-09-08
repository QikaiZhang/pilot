// Package controller 提供 HTTP 接口层。
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"Pilot/internal/deps"
)

const healthTimeout = 3 * time.Second

// HealthHandler 提供 /api/v1/health/live 与 /api/v1/health/ready。
type HealthHandler struct {
	checkers []deps.Dependency
}

// NewHealthHandler 构造健康检查处理器。
func NewHealthHandler(checkers []deps.Dependency) *HealthHandler {
	return &HealthHandler{checkers: checkers}
}

// Live 只证明进程存在。
func (h *HealthHandler) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready 逐项报告依赖状态；全部可用返回 200，否则 503。
// WARNING S级核心逻辑：理解后必须删除本区域，并从零独立重写，禁止直接复用。
// 删除范围：从 BEGIN S_LEVEL_REFERENCE 到 END S_LEVEL_REFERENCE。
// BEGIN S_LEVEL_REFERENCE
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
	defer cancel()

	results := deps.CheckAll(ctx, h.checkers)
	status, code := "ok", http.StatusOK
	if !deps.AllOK(results) {
		status, code = "unavailable", http.StatusServiceUnavailable
	}

	writeJSON(w, code, readyResponse{
		Status: status,
		Checks: toCheckViews(results),
	})
}

// END S_LEVEL_REFERENCE

type readyResponse struct {
	Status string      `json:"status"`
	Checks []checkView `json:"checks"`
}

type checkView struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

func toCheckViews(results []deps.Result) []checkView {
	views := make([]checkView, 0, len(results))
	for _, r := range results {
		status := "ok"
		if !r.OK {
			status = "error"
		}
		view := checkView{
			Name:      r.Name,
			Status:    status,
			LatencyMS: r.LatencyMS,
		}
		if r.Err != nil {
			view.Error = r.Err.Error()
		}
		views = append(views, view)
	}
	return views
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteError 写统一错误响应体。
func WriteError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Code: code, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
