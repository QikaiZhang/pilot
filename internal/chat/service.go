package chat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"Pilot/internal/ai"
	"Pilot/internal/memory"
)

var (
	ErrMemoryStoreRequired = errors.New("chat memory store is required")
	ErrChatModelRequired   = errors.New("chat model is required")
	ErrInvalidHistoryLimit = errors.New("chat history limit must be positive")
	ErrInvalidTurn         = errors.New("user id, session id, and query are required")
)

type memoryStore interface {
	LoadRecent(ctx context.Context, userID, sessionID string, limit int) ([]memory.ChatMessage, error)
	SaveMessages(ctx context.Context, messages []memory.ChatMessage) error
}

// Config 保存一轮聊天的服务端策略。
type Config struct {
	HistoryLimit int
	SystemPrompt string
}

// Service 负责协调会话记忆和供应商无关的 AI 模型。
type Service struct {
	memory memoryStore
	model  ai.ChatModel
	config Config
	now    func() time.Time
	newID  func() (string, error)
}

func NewService(memoryStore memoryStore, model ai.ChatModel, config Config) (*Service, error) {
	if memoryStore == nil {
		return nil, ErrMemoryStoreRequired
	}
	if model == nil {
		return nil, ErrChatModelRequired
	}
	if config.HistoryLimit <= 0 {
		return nil, ErrInvalidHistoryLimit
	}

	return &Service{
		memory: memoryStore,
		model:  model,
		config: config,
		now:    func() time.Time { return time.Now().UTC() },
		newID:  newMessageID,
	}, nil
}

// Generate 完成一轮同步对话。
// 用户消息在调用模型前持久化；只有拿到完整模型响应后才持久化助手消息。
func (s *Service) Generate(ctx context.Context, request TurnRequest) (ChatResponse, error) {
	normalized, err := normalizeTurn(request)
	if err != nil {
		return ChatResponse{}, err
	}
	request = normalized

	history, err := s.memory.LoadRecent(ctx, request.UserID, request.SessionID, s.config.HistoryLimit)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("load recent conversation: %w", err)
	}
	
	userMessage, err := s.newMemoryMessage(request, memory.RoleUser, request.Query)
	if err != nil {
		return ChatResponse{}, err
	}
	if err := s.memory.SaveMessages(ctx, []memory.ChatMessage{userMessage}); err != nil {
		return ChatResponse{}, fmt.Errorf("save user message: %w", err)
	}
	//真正调用逻辑
	response, err := s.model.Generate(ctx, ai.ModelRequest{
		Messages: buildModelMessages(s.config.SystemPrompt, history, userMessage),
	})
	if err != nil {
		return ChatResponse{}, fmt.Errorf("generate chat response: %w", err)
	}
	if response.Message.Role != ai.RoleAssistant || strings.TrimSpace(response.Message.Content) == "" {
		return ChatResponse{}, errors.New("chat model returned an invalid assistant response")
	}

	//拿到 assitantMessage
	assistantMessage, err := s.newMemoryMessage(request, memory.RoleAssistant, response.Message.Content)
	if err != nil {
		return ChatResponse{}, err
	}
	if err := s.memory.SaveMessages(ctx, []memory.ChatMessage{assistantMessage}); err != nil {
		return ChatResponse{}, fmt.Errorf("save assistant message: %w", err)
	}
	
	return ChatResponse{Content: response.Message.Content, Usage: response.Usage}, nil
}

func (s *Service) newMemoryMessage(request TurnRequest, role memory.Role, content string) (memory.ChatMessage, error) {
	id, err := s.newID()
	if err != nil {
		return memory.ChatMessage{}, fmt.Errorf("generate message id: %w", err)
	}
	return memory.ChatMessage{
		ID:        id,
		UserID:    request.UserID,
		SessionID: request.SessionID,
		Role:      role,
		Content:   content,
		CreatedAt: s.now(),
	}, nil
}

func buildModelMessages(systemPrompt string, history []memory.ChatMessage, userMessage memory.ChatMessage) []ai.Message {
	messages := make([]ai.Message, 0, len(history)+2)
	if prompt := strings.TrimSpace(systemPrompt); prompt != "" {
		messages = append(messages, ai.Message{Role: ai.RoleSystem, Content: prompt})
	}
	for _, message := range history {
		messages = append(messages, ai.Message{
			Role:    ai.Role(message.Role),
			Content: message.Content,
		})
	}
	messages = append(messages, ai.Message{Role: ai.RoleUser, Content: userMessage.Content})
	return messages
}

func validateTurn(request TurnRequest) error {
	_, err := normalizeTurn(request)
	return err
}

func normalizeTurn(request TurnRequest) (TurnRequest, error) {
	request.UserID = strings.TrimSpace(request.UserID)
	request.SessionID = strings.TrimSpace(request.SessionID)
	request.Query = strings.TrimSpace(request.Query)
	if strings.TrimSpace(request.UserID) == "" || strings.TrimSpace(request.SessionID) == "" || strings.TrimSpace(request.Query) == "" {
		return TurnRequest{}, ErrInvalidTurn
	}
	return request, nil
}

func newMessageID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
