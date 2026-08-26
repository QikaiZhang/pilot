package eino

import (
	"context"
	"errors"
	"testing"

	projectai "Pilot/internal/ai"

	"github.com/cloudwego/eino/schema"
)

func TestNewChatModelValidatesConfig(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   error
	}{
		{name: "missing api key", config: Config{Model: "model"}, want: ErrAPIKeyRequired},
		{name: "missing model", config: Config{APIKey: "secret"}, want: ErrModelRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewChatModel(context.Background(), tt.config)
			if !errors.Is(err, tt.want) {
				t.Fatalf("NewChatModel() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestToEinoMessagesMapsRolesAndContent(t *testing.T) {
	got, err := toEinoMessages([]projectai.Message{
		{Role: projectai.RoleSystem, Content: "system"},
		{Role: projectai.RoleUser, Content: "question"},
		{Role: projectai.RoleAssistant, Content: "answer"},
		{Role: projectai.RoleTool, Content: "tool result"},
	})
	if err != nil {
		t.Fatalf("toEinoMessages() error = %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(toEinoMessages()) = %d, want 4", len(got))
	}
	if string(got[1].Role) != string(projectai.RoleUser) || got[1].Content != "question" {
		t.Fatalf("mapped message = %+v", got[1])
	}
}

func TestToEinoMessagesRejectsInvalidRoleAndEmptyContent(t *testing.T) {
	tests := []struct {
		name    string
		message projectai.Message
	}{
		{name: "invalid role", message: projectai.Message{Role: projectai.Role("unknown"), Content: "text"}},
		{name: "empty content", message: projectai.Message{Role: projectai.RoleUser, Content: "  "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := toEinoMessages([]projectai.Message{tt.message}); err == nil {
				t.Fatal("toEinoMessages() returned nil error")
			}
		})
	}
}

func TestFromEinoResponseMapsContentAndUsage(t *testing.T) {
	response, err := fromEinoResponse(&schema.Message{
		Role:    schema.Assistant,
		Content: "answer",
		ResponseMeta: &schema.ResponseMeta{
			Usage: &schema.TokenUsage{PromptTokens: 12, CompletionTokens: 7},
		},
	})
	if err != nil {
		t.Fatalf("fromEinoResponse() error = %v", err)
	}
	if response.Message.Role != projectai.RoleAssistant || response.Message.Content != "answer" {
		t.Fatalf("response.Message = %+v", response.Message)
	}
	if response.Usage.InputTokens != 12 || response.Usage.OutputTokens != 7 {
		t.Fatalf("response.Usage = %+v", response.Usage)
	}
}

func TestFromEinoResponseRejectsNil(t *testing.T) {
	if _, err := fromEinoResponse(nil); err == nil {
		t.Fatal("fromEinoResponse(nil) returned nil error")
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	if got := normalizeBaseURL("https://example.test/v1/chat/completions/"); got != "https://example.test/v1" {
		t.Fatalf("normalizeBaseURL() = %q", got)
	}
}
