package memory

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type ChatMessage struct {
	ID        string         `json:"id"`
	UserID    string         `json:"user_id"`
	SessionID string         `json:"session_id"`
	Content   string         `json:"content"`
	CreatedAt time.Time      `json:"created_at"`
	Role      Role           `json:"role"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

func (r Role) Valid() bool {
	switch r {
	case RoleUser, RoleAssistant, RoleSystem, RoleTool:
		return true
	default:
		return false
	}
}

func (m ChatMessage) Validate() error {
	if strings.TrimSpace(m.ID) == "" {
		return errors.New("message id is required")
	}
	if strings.TrimSpace(m.UserID) == "" {
		return errors.New("user id is required")
	}
	if strings.TrimSpace(m.SessionID) == "" {
		return errors.New("session id is required")
	}
	if !m.Role.Valid() {
		return fmt.Errorf("invalid message role: %q", m.Role)
	}
	if strings.TrimSpace(m.Content) == "" {
		return errors.New("message content is required")
	}
	return nil
}
