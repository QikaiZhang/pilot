package tools

import (
	"context"
	"encoding/json"
	"testing"

	"Pilot/internal/ai"
)

type fakeRetriever struct {
	chunks    []ai.Chunk
	err       error
	lastQuery string
	lastTopK  int
}

func (f *fakeRetriever) Retrieve(_ context.Context, query string, topK int) ([]ai.Chunk, error) {
	f.lastQuery, f.lastTopK = query, topK
	if f.err != nil {
		return nil, f.err
	}
	return f.chunks, nil
}

func TestKnowledgeSearchExecute(t *testing.T) {
	retriever := &fakeRetriever{chunks: []ai.Chunk{{ChunkID: "c1", Content: "片段"}}}
	tool := NewKnowledgeSearch(retriever)

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"Redis 超时","top_k":5}`))
	if err != nil {
		t.Fatal(err)
	}
	result, ok := out.(KnowledgeSearchResult)
	if !ok {
		t.Fatalf("output type = %T, want KnowledgeSearchResult", out)
	}
	if result.HitCount != 1 || result.Query != "Redis 超时" {
		t.Fatalf("result = %+v, want 1 hit", result)
	}
	if retriever.lastQuery != "Redis 超时" || retriever.lastTopK != 5 {
		t.Fatalf("retriever call = %q/%d, want Redis 超时/5", retriever.lastQuery, retriever.lastTopK)
	}
}

func TestKnowledgeSearchEmptyQueryFails(t *testing.T) {
	tool := NewKnowledgeSearch(&fakeRetriever{})
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"query":""}`)); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestKnowledgeSearchFilterByCategory(t *testing.T) {
	retriever := &fakeRetriever{chunks: []ai.Chunk{
		{ChunkID: "c1", Category: "数据库"},
		{ChunkID: "c2", Category: "缓存"},
		{ChunkID: "c3", Category: "数据库"},
	}}
	tool := NewKnowledgeSearch(retriever)

	out, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"分库","category":"数据库","top_k":10}`))
	if err != nil {
		t.Fatal(err)
	}
	result := out.(KnowledgeSearchResult)
	if result.HitCount != 2 {
		t.Fatalf("filtered hit count = %d, want 2 (only 数据库)", result.HitCount)
	}
}
