package memory

import (
	"context"
	"time"
)

type HistoryStore interface {
	Append(ctx context.Context, messages []ChatMessage) error
	List(ctx context.Context, userID string, sessionID string, limit int) ([]ChatMessage, error)
	Delete(ctx context.Context, userID string, sessionID string) error
}

// Redis存储近期记忆
type RecentMemory interface {
	Get(ctx context.Context, userID, sessionID string, limit int) ([]ChatMessage, error)
	Append(ctx context.Context, messages []ChatMessage, limit int, ttl time.Duration) error
	Clear(ctx context.Context, userID, sessionID string) error
}
