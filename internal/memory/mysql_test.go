package memory

import (
	"testing"
	"time"
)

func TestToInsertParamsSetsCreatedAtForZeroValue(t *testing.T) {
	before := time.Now().UTC()
	params, err := toInsertParams(ChatMessage{
		ID:        "message-1",
		UserID:    "user-1",
		SessionID: "session-1",
		Role:      RoleUser,
		Content:   "check redis",
	})
	if err != nil {
		t.Fatalf("toInsertParams() error = %v", err)
	}
	after := time.Now().UTC()

	if params.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero")
	}
	if params.CreatedAt.Before(before) || params.CreatedAt.After(after) {
		t.Fatalf("CreatedAt = %v, want between %v and %v", params.CreatedAt, before, after)
	}
}

func TestToInsertParamsPreservesCreatedAt(t *testing.T) {
	createdAt := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	params, err := toInsertParams(ChatMessage{
		ID:        "message-1",
		UserID:    "user-1",
		SessionID: "session-1",
		Role:      RoleAssistant,
		Content:   "answer",
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("toInsertParams() error = %v", err)
	}
	if !params.CreatedAt.Equal(createdAt) {
		t.Fatalf("CreatedAt = %v, want %v", params.CreatedAt, createdAt)
	}
}

func TestNewMySQLStoreRejectsNilDB(t *testing.T) {
	if _, err := NewMySQLStore(nil); err == nil {
		t.Fatal("NewMySQLStore(nil) returned nil error")
	}
}
