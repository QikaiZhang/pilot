package memory

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const recentMemoryPrefix = "pilot:memory:"

var (
	ErrStoreNotInitialized = errors.New("memory store is not initialized")
	ErrEmptyMessages       = errors.New("messages cannot be empty")
	ErrInvalidLimit        = errors.New("memory limit must be positive")
	ErrInvalidTTL          = errors.New("memory ttl must be positive")
	ErrSessionMismatch     = errors.New("messages belong to different sessions")
)

// RedisStore keeps a bounded, expiring context window for each conversation.
// MySQLStore remains the durable source of truth.
type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(client *redis.Client) (*RedisStore, error) {
	if client == nil {
		return nil, errors.New("redis client is nil")
	}
	return &RedisStore{client: client}, nil
}

func (s *RedisStore) Get(ctx context.Context, userID, sessionID string, limit int) ([]ChatMessage, error) {
	if s == nil || s.client == nil {
		return nil, ErrStoreNotInitialized
	}
	if limit <= 0 {
		return nil, ErrInvalidLimit
	}

	values, err := s.client.LRange(ctx, recentMemoryKey(userID, sessionID), int64(-limit), -1).Result()
	if err != nil {
		return nil, fmt.Errorf("read recent memory: %w", err)
	}

	messages := make([]ChatMessage, 0, len(values))
	for _, value := range values {
		var message ChatMessage
		if err := json.Unmarshal([]byte(value), &message); err != nil {
			return nil, fmt.Errorf("decode recent memory message: %w", err)
		}
		if err := message.Validate(); err != nil {
			return nil, fmt.Errorf("decode recent memory message: %w", err)
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (s *RedisStore) Append(ctx context.Context, messages []ChatMessage, limit int, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return ErrStoreNotInitialized
	}
	if len(messages) == 0 {
		return ErrEmptyMessages
	}
	if limit <= 0 {
		return ErrInvalidLimit
	}
	if ttl <= 0 {
		return ErrInvalidTTL
	}

	first := messages[0]
	values := make([]any, 0, len(messages))
	for i := range messages {
		message := messages[i]
		if err := message.Validate(); err != nil {
			return fmt.Errorf("validate recent memory message %q: %w", message.ID, err)
		}
		if message.UserID != first.UserID || message.SessionID != first.SessionID {
			return ErrSessionMismatch
		}
		if message.CreatedAt.IsZero() {
			message.CreatedAt = time.Now().UTC()
		}
		messageBytes, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("marshal recent memory message %q: %w", message.ID, err)
		}
		values = append(values, string(messageBytes))
	}
	//important：redisPipeLine
	key := recentMemoryKey(first.UserID, first.SessionID)
	if _, err := s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.RPush(ctx, key, values...)
		pipe.LTrim(ctx, key, -int64(limit), -1)
		pipe.Expire(ctx, key, ttl)
		return nil
	}); err != nil {
		return fmt.Errorf("append recent memory: %w", err)
	}

	return nil
}

func (s *RedisStore) Clear(ctx context.Context, userID, sessionID string) error {
	if s == nil || s.client == nil {
		return ErrStoreNotInitialized
	}
	if err := s.client.Del(ctx, recentMemoryKey(userID, sessionID)).Err(); err != nil {
		return fmt.Errorf("clear recent memory: %w", err)
	}
	return nil
}

func recentMemoryKey(userID, sessionID string) string {
	// Encoded components prevent ':' in user-controlled values from causing key collisions.
	return recentMemoryPrefix + base64.RawURLEncoding.EncodeToString([]byte(userID)) + ":" + base64.RawURLEncoding.EncodeToString([]byte(sessionID))
}
