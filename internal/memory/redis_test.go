package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRecentMemoryKeySeparatesEncodedComponents(t *testing.T) {
	first := recentMemoryKey("a:b", "c")
	second := recentMemoryKey("a", "b:c")
	if first == second {
		t.Fatalf("keys collided: %q", first)
	}
}

func TestRedisAppendRejectsMixedSessionsBeforeNetworkCall(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	store, err := NewRedisStore(client)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Append(context.Background(), []ChatMessage{
		{ID: "m1", UserID: "u1", SessionID: "s1", Role: RoleUser, Content: "question"},
		{ID: "m2", UserID: "u1", SessionID: "s2", Role: RoleAssistant, Content: "answer"},
	}, 20, time.Minute)
	if !errors.Is(err, ErrSessionMismatch) {
		t.Fatalf("Append() error = %v, want ErrSessionMismatch before network call", err)
	}
}

func TestRedisAppendRejectsInvalidMessage(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	store, err := NewRedisStore(client)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Append(context.Background(), []ChatMessage{{
		ID:        "m1",
		UserID:    "u1",
		SessionID: "s1",
		Role:      Role("unknown"),
		Content:   "question",
	}}, 20, 1)
	if err == nil {
		t.Fatal("Append() returned nil for invalid role")
	}
}
