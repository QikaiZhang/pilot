package memory

import (
	"strings"
	"testing"
)

func TestRoleValid(t *testing.T) {
	tests := []struct {
		name  string
		role  Role
		valid bool
	}{
		{name: "user", role: RoleUser, valid: true},
		{name: "assistant", role: RoleAssistant, valid: true},
		{name: "system", role: RoleSystem, valid: true},
		{name: "tool", role: RoleTool, valid: true},
		{name: "unknown", role: Role("moderator"), valid: false},
		{name: "empty", role: Role(""), valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.Valid(); got != tt.valid {
				t.Fatalf("Role(%q).Valid() = %t, want %t", tt.role, got, tt.valid)
			}
		})
	}
}

func TestChatMessageValidate(t *testing.T) {
	valid := ChatMessage{
		ID:        "message-1",
		UserID:    "user-1",
		SessionID: "session-1",
		Role:      RoleUser,
		Content:   "check redis",
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("valid message rejected: %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*ChatMessage)
		wantErr string
	}{
		{name: "missing id", mutate: func(m *ChatMessage) { m.ID = "" }, wantErr: "message id is required"},
		{name: "missing user", mutate: func(m *ChatMessage) { m.UserID = " " }, wantErr: "user id is required"},
		{name: "missing session", mutate: func(m *ChatMessage) { m.SessionID = "" }, wantErr: "session id is required"},
		{name: "invalid role", mutate: func(m *ChatMessage) { m.Role = Role("unknown") }, wantErr: "invalid message role"},
		{name: "blank content", mutate: func(m *ChatMessage) { m.Content = " \n\t" }, wantErr: "message content is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := valid
			tt.mutate(&message)
			err := message.Validate()
			if err == nil {
				t.Fatal("Validate() returned nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}
