package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	projectagent "Pilot/internal/agent"
	"Pilot/internal/ai"
	"Pilot/internal/memory"
)

var (
	ErrAgentRunnerRequired = errors.New("agent runner is required")
	ErrAgentAnswerEmpty    = errors.New("agent returned an empty answer")
)

// AgentService 负责把会话记忆与 Agent 执行串成一个完整业务用例。
// Agent 本身不访问记忆存储；历史准备和结果持久化都由这一层完成。
type AgentService struct {
	memory memoryStore
	runner projectagent.Runner
	config Config
	now    func() time.Time
	newID  func() (string, error)
}

// NewAgentService 创建 Agent 对话服务。
func NewAgentService(memoryStore memoryStore, runner projectagent.Runner, config Config) (*AgentService, error) {
	if memoryStore == nil {
		return nil, ErrMemoryStoreRequired
	}
	if runner == nil {
		return nil, ErrAgentRunnerRequired
	}
	if config.HistoryLimit <= 0 {
		return nil, ErrInvalidHistoryLimit
	}
	return &AgentService{
		memory: memoryStore,
		runner: runner,
		config: config,
		now:    func() time.Time { return time.Now().UTC() },
		newID:  newMessageID,
	}, nil
}

// Run 执行一轮 Agent 对话，并只在得到最终答案后保存助手消息。
func (s *AgentService) Run(ctx context.Context, request TurnRequest) (projectagent.AgentResponse, error) {
	if s == nil || s.runner == nil {
		return projectagent.AgentResponse{}, ErrAgentRunnerRequired
	}
	normalized, err := normalizeTurn(request)
	if err != nil {
		return projectagent.AgentResponse{}, err
	}
	request = normalized

	history, err := s.memory.LoadRecent(ctx, request.UserID, request.SessionID, s.config.HistoryLimit)
	if err != nil {
		return projectagent.AgentResponse{}, fmt.Errorf("load recent conversation for agent: %w", err)
	}
	userMessage, err := s.newMemoryMessage(request, memory.RoleUser, request.Query)
	if err != nil {
		return projectagent.AgentResponse{}, err
	}
	if err := s.memory.SaveMessages(ctx, []memory.ChatMessage{userMessage}); err != nil {
		return projectagent.AgentResponse{}, fmt.Errorf("save agent user message: %w", err)
	}

	response, err := s.runner.Run(ctx, projectagent.AgentRequest{
		UserID:    request.UserID,
		SessionID: request.SessionID,
		Query:     request.Query,
		History:   agentHistory(s.config.SystemPrompt, history),
	})
	if err != nil {
		return projectagent.AgentResponse{}, fmt.Errorf("run agent: %w", err)
	}
	if strings.TrimSpace(response.Answer) == "" {
		return projectagent.AgentResponse{}, ErrAgentAnswerEmpty
	}

	assistantMessage, err := s.newMemoryMessage(request, memory.RoleAssistant, response.Answer)
	if err != nil {
		return projectagent.AgentResponse{}, err
	}
	if err := s.memory.SaveMessages(ctx, []memory.ChatMessage{assistantMessage}); err != nil {
		return projectagent.AgentResponse{}, fmt.Errorf("save agent assistant message: %w", err)
	}
	response.Answer = strings.TrimSpace(response.Answer)
	return response, nil
}

func (s *AgentService) newMemoryMessage(request TurnRequest, role memory.Role, content string) (memory.ChatMessage, error) {
	id, err := s.newID()
	if err != nil {
		return memory.ChatMessage{}, fmt.Errorf("generate agent message id: %w", err)
	}
	return memory.ChatMessage{ID: id, UserID: request.UserID, SessionID: request.SessionID, Role: role, Content: content, CreatedAt: s.now()}, nil
}

func agentHistory(systemPrompt string, history []memory.ChatMessage) []ai.Message {
	messages := make([]ai.Message, 0, len(history)+1)
	if prompt := strings.TrimSpace(systemPrompt); prompt != "" {
		messages = append(messages, ai.Message{Role: ai.RoleSystem, Content: prompt})
	}
	for _, message := range history {
		messages = append(messages, ai.Message{Role: ai.Role(message.Role), Content: message.Content})
	}
	return messages
}
