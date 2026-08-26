package chat

import (
	"context"
	"errors"
	"io"
	"testing"

	"Pilot/internal/ai"
	"Pilot/internal/memory"
)

type streamModel struct {
	stream ai.TokenStream
	seen   []ai.ModelRequest
}

func (m *streamModel) Generate(context.Context, ai.ModelRequest) (ai.ModelResponse, error) {
	return ai.ModelResponse{}, errors.New("generate is not used in stream test")
}

func (m *streamModel) Stream(_ context.Context, request ai.ModelRequest) (ai.TokenStream, error) {
	m.seen = append(m.seen, request)
	return m.stream, nil
}

type streamTokenStream struct {
	chunks    []ai.ModelChunk
	index     int
	closeCall int
	err       error
}

func (s *streamTokenStream) Recv() (ai.ModelChunk, error) {
	if s.index < len(s.chunks) {
		chunk := s.chunks[s.index]
		s.index++
		return chunk, nil
	}
	if s.err != nil {
		err := s.err
		s.err = nil
		return ai.ModelChunk{}, err
	}
	return ai.ModelChunk{}, io.EOF
}

func (s *streamTokenStream) Close() error {
	s.closeCall++
	return nil
}

func TestServiceStreamSavesAssistantOnlyAfterEOF(t *testing.T) {
	memoryStore := &fakeMemoryStore{}
	upstream := &streamTokenStream{chunks: []ai.ModelChunk{
		{Delta: "你好"},
		{Delta: "，世界", Usage: &ai.Usage{InputTokens: 3, OutputTokens: 2}},
	}}
	model := &streamModel{stream: upstream}
	service := newTestService(t, memoryStore, &fakeChatModel{})
	service.model = model

	stream, err := service.Stream(context.Background(), TurnRequest{UserID: " u1 ", SessionID: " s1 ", Query: " hello "})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if len(memoryStore.saved) != 1 || memoryStore.saved[0][0].Role != memory.RoleUser {
		t.Fatalf("saved before receive = %+v, want only user message", memoryStore.saved)
	}

	event, err := stream.Recv()
	if err != nil || event.Type != StreamEventContent || event.Content != "你好" {
		t.Fatalf("first Recv() = %+v, %v", event, err)
	}
	if len(memoryStore.saved) != 1 {
		t.Fatalf("saved after first chunk = %d, want 1", len(memoryStore.saved))
	}

	event, err = stream.Recv()
	if err != nil || event.Type != StreamEventContent || event.Content != "，世界" {
		t.Fatalf("second Recv() = %+v, %v", event, err)
	}
	event, err = stream.Recv()
	if err != nil || event.Type != StreamEventDone || event.Usage == nil || event.Usage.OutputTokens != 2 {
		t.Fatalf("done Recv() = %+v, %v", event, err)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("final Recv() error = %v, want EOF", err)
	}

	if len(memoryStore.saved) != 2 || memoryStore.saved[1][0].Role != memory.RoleAssistant || memoryStore.saved[1][0].Content != "你好，世界" {
		t.Fatalf("saved messages = %+v, want user then complete assistant", memoryStore.saved)
	}
	if upstream.closeCall != 1 {
		t.Fatalf("upstream Close() calls = %d, want 1", upstream.closeCall)
	}
	if len(model.seen) != 1 || model.seen[0].Messages[len(model.seen[0].Messages)-1].Content != "hello" {
		t.Fatalf("model request = %+v, want normalized query", model.seen)
	}
}

func TestServiceStreamDoesNotSaveAssistantAfterUpstreamError(t *testing.T) {
	memoryStore := &fakeMemoryStore{}
	upstream := &streamTokenStream{chunks: []ai.ModelChunk{{Delta: "partial"}}, err: errors.New("upstream unavailable")}
	service := newTestService(t, memoryStore, &fakeChatModel{})
	service.model = &streamModel{stream: upstream}

	stream, err := service.Stream(context.Background(), TurnRequest{UserID: "u1", SessionID: "s1", Query: "hello"})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	_, _ = stream.Recv()
	event, err := stream.Recv()
	if err != nil || event.Type != StreamEventError || event.Error == nil || event.Error.Code != "UPSTREAM_ERROR" {
		t.Fatalf("error Recv() = %+v, %v", event, err)
	}
	if len(memoryStore.saved) != 1 || memoryStore.saved[0][0].Role != memory.RoleUser {
		t.Fatalf("saved messages = %+v, want only user message", memoryStore.saved)
	}
}

func TestServiceStreamCancellationDoesNotSaveAssistant(t *testing.T) {
	memoryStore := &fakeMemoryStore{}
	upstream := &streamTokenStream{}
	service := newTestService(t, memoryStore, &fakeChatModel{})
	service.model = &streamModel{stream: upstream}
	ctx, cancel := context.WithCancel(context.Background())

	stream, err := service.Stream(ctx, TurnRequest{UserID: "u1", SessionID: "s1", Query: "hello"})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	cancel()
	if _, err := stream.Recv(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Recv() error = %v, want context.Canceled", err)
	}
	if len(memoryStore.saved) != 1 || memoryStore.saved[0][0].Role != memory.RoleUser {
		t.Fatalf("saved messages = %+v, want only user message", memoryStore.saved)
	}
	if upstream.closeCall != 1 {
		t.Fatalf("upstream Close() calls = %d, want 1", upstream.closeCall)
	}
}
