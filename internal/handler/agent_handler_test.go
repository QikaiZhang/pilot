package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	projectagent "Pilot/internal/agent"
	"Pilot/internal/chat"
	"Pilot/internal/memory"
)

type agentRunnerStub struct{}

func (agentRunnerStub) Run(context.Context, projectagent.AgentRequest) (projectagent.AgentResponse, error) {
	return projectagent.AgentResponse{Answer: "agent ok"}, nil
}

func newTestAgentHandler(t *testing.T) *AgentHandler {
	t.Helper()
	service, err := chat.NewAgentService(agentHandlerMemoryStub{}, agentRunnerStub{}, chat.Config{HistoryLimit: 2})
	if err != nil {
		t.Fatal(err)
	}
	return NewAgentHandler(service)
}

type agentHandlerMemoryStub struct{}

func (agentHandlerMemoryStub) LoadRecent(context.Context, string, string, int) ([]memory.ChatMessage, error) {
	return nil, nil
}

func (agentHandlerMemoryStub) SaveMessages(context.Context, []memory.ChatMessage) error { return nil }

func TestAgentHandlerRun(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/chat", strings.NewReader(`{"user_id":"u1","session_id":"s1","query":"hello"}`))

	newTestAgentHandler(t).Run(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), `"answer":"agent ok"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestAgentHandlerReturnsUnavailableWithoutService(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/chat", strings.NewReader(`{}`))

	NewAgentHandler(nil).Run(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

type agentTimeoutRunnerStub struct{}

func (agentTimeoutRunnerStub) Run(context.Context, projectagent.AgentRequest) (projectagent.AgentResponse, error) {
	return projectagent.AgentResponse{}, projectagent.ErrAgentTimeout
}

func TestAgentHandlerMapsRunnerTimeoutToGatewayTimeout(t *testing.T) {
	service, err := chat.NewAgentService(agentHandlerMemoryStub{}, agentTimeoutRunnerStub{}, chat.Config{HistoryLimit: 2})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/chat", strings.NewReader(`{"user_id":"u1","session_id":"s1","query":"hello"}`))

	NewAgentHandler(service).Run(recorder, request)

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusGatewayTimeout)
	}
	if !strings.Contains(recorder.Body.String(), `"code":"agent_timeout"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}
