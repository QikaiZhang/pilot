package ai

import (
	"context"
	"errors"
	"io"
	"strings"
)

// MockChatModel 是用于本地开发和测试的确定性模型。
type MockChatModel struct {
	GenerateError error
	StreamError   error
}

func (m MockChatModel) Generate(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	if err := ctx.Err(); err != nil {
		return ModelResponse{}, err
	}
	if m.GenerateError != nil {
		return ModelResponse{}, m.GenerateError
	}

	query := lastUserMessage(request.Messages)
	if query == "" {
		return ModelResponse{}, errors.New("model request has no user message")
	}
	return ModelResponse{
		Message: Message{Role: RoleAssistant, Content: "已收到：" + query},
		Usage:   Usage{InputTokens: len([]rune(query)), OutputTokens: len([]rune(query)) + 4},
	}, nil
}

func (m MockChatModel) Stream(ctx context.Context, request ModelRequest) (TokenStream, error) {
	response, err := m.Generate(ctx, request)
	if err != nil {
		return nil, err
	}
	chunks := strings.Fields(response.Message.Content)
	return &mockTokenStream{ctx: ctx, chunks: chunks, streamErr: m.StreamError}, nil
}

func lastUserMessage(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

type mockTokenStream struct {
	ctx       context.Context
	chunks    []string
	streamErr error
	index     int
	closed    bool
}

func (s *mockTokenStream) Recv() (ModelChunk, error) {
	if s.closed {
		return ModelChunk{}, io.EOF
	}
	if err := s.ctx.Err(); err != nil {
		return ModelChunk{}, err
	}
	if s.index < len(s.chunks) {
		chunk := ModelChunk{Delta: s.chunks[s.index]}
		s.index++
		return chunk, nil
	}
	if s.streamErr != nil {
		err := s.streamErr
		s.streamErr = nil
		return ModelChunk{}, err
	}
	return ModelChunk{}, io.EOF
}

func (s *mockTokenStream) Close() error {
	s.closed = true
	return nil
}
