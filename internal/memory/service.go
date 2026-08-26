package memory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

var (
	ErrHistoryStoreRequired = errors.New("history store is required")
	ErrRecentMemoryRequired = errors.New("recent memory is required")
	ErrInvalidRecentLimit   = errors.New("recent limit must be positive")
	ErrInvalidRecentTTL     = errors.New("recent ttl must be positive")
	ErrInvalidSessionID     = errors.New("user id and session id are required")
)

// ServiceConfig contains the policy used when the service updates recent memory.
type ServiceConfig struct {
	RecentLimit int
	RecentTTL   time.Duration
}

// Service coordinates durable conversation history and the recent-memory cache.
// It does not know SQL or Redis details; those remain behind the ports.
type Service struct {
	history HistoryStore
	recent  RecentMemory
	config  ServiceConfig
}

func NewService(history HistoryStore, recent RecentMemory, config ServiceConfig) (*Service, error) {
	if history == nil {
		return nil, ErrHistoryStoreRequired
	}
	if recent == nil {
		return nil, ErrRecentMemoryRequired
	}
	if config.RecentLimit <= 0 {
		return nil, ErrInvalidRecentLimit
	}
	if config.RecentTTL <= 0 {
		return nil, ErrInvalidRecentTTL
	}

	return &Service{
		history: history,
		recent:  recent,
		config:  config,
	}, nil
}
func (s *Service) LoadRecent(
	ctx context.Context,
	userID string,
	sessionID string,
	limit int,
) ([]ChatMessage, error) {
	if s == nil || s.history == nil || s.recent == nil {
		return nil, ErrHistoryStoreRequired
	}
	// Service 不能假设调用方一定是 HTTP Handler，因此保留领域边界校验。
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(sessionID) == "" {
		return nil, ErrInvalidSessionID
	}
	if limit <= 0 {
		return nil, ErrInvalidLimit
	}

	// 先查近期记忆，命中时不访问持久层。
	messages, err := s.recent.Get(ctx, userID, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("get recent memory: %w", err)
	}
	if len(messages) > 0 {
		return messages, nil
	}

	// 缓存未命中时回源 MySQL；空历史是正常结果。
	messages, err = s.history.List(ctx, userID, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("list conversation history: %w", err)
	}
	if len(messages) == 0 {
		return messages, nil
	}

	// 回填失败不影响已经从 MySQL 读到的结果，但必须留下可观测记录。
	if err := s.recent.Append(ctx, messages, limit, s.config.RecentTTL); err != nil {
		slog.WarnContext(ctx, "refresh recent memory failed", "user_id", userID, "session_id", sessionID, "error", err)
	}
	return messages, nil
}

// SaveMessages persists a message batch and then refreshes recent memory.
// MySQL is the durable source of truth; a Redis failure does not undo a
// successful durable write.
func (s *Service) SaveMessages(ctx context.Context, messages []ChatMessage) error {
	if s == nil || s.history == nil || s.recent == nil {
		return ErrHistoryStoreRequired
	}
	if err := validateMessageBatch(messages); err != nil {
		return err
	}

	if err := s.history.Append(ctx, messages); err != nil {
		return fmt.Errorf("save conversation history: %w", err)
	}

	if err := s.recent.Append(ctx, messages, s.config.RecentLimit, s.config.RecentTTL); err != nil {
		slog.WarnContext(ctx, "refresh recent memory after save failed",
			"user_id", messages[0].UserID,
			"session_id", messages[0].SessionID,
			"error", err,
		)
	}
	return nil
}

func validateMessageBatch(messages []ChatMessage) error {
	if len(messages) == 0 {
		return ErrEmptyMessages
	}

	first := messages[0]
	for _, message := range messages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("validate message %q: %w", message.ID, err)
		}
		if message.UserID != first.UserID || message.SessionID != first.SessionID {
			return ErrSessionMismatch
		}
	}
	return nil
}
