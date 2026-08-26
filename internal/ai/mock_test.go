package ai

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestMockChatModelGenerateUsesLastUserMessage(t *testing.T) {
	model := MockChatModel{}
	response, err := model.Generate(context.Background(), ModelRequest{Messages: []Message{
		{Role: RoleSystem, Content: "system"},
		{Role: RoleUser, Content: "first"},
		{Role: RoleAssistant, Content: "answer"},
		{Role: RoleUser, Content: "last query"},
	}})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if response.Message.Role != RoleAssistant || response.Message.Content != "已收到：last query" {
		t.Fatalf("Generate() = %+v", response)
	}
}

func TestMockChatModelStreamReturnsEOF(t *testing.T) {
	model := MockChatModel{}
	stream, err := model.Stream(context.Background(), ModelRequest{Messages: []Message{{Role: RoleUser, Content: "hello world"}}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	for {
		_, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("Recv() error = %v", err)
		}
	}
}
