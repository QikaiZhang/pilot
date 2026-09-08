package retriever

import (
	"testing"

	"Pilot/internal/ai"
)

func TestSplitDocumentUsesStableVersionedChunkIDs(t *testing.T) {
	document := KnowledgeDocument{DocID: "redis-timeout", Title: "Redis 超时", Content: "第一段内容。第二段内容。第三段内容。", Version: 2}
	first := SplitDocument(document, 10, 2)
	second := SplitDocument(document, 10, 2)
	if len(first) < 2 {
		t.Fatalf("chunks = %+v, want multiple chunks", first)
	}
	for index := range first {
		if first[index].ChunkID != second[index].ChunkID || first[index].Version != 2 || first[index].DocID != "redis-timeout" {
			t.Fatalf("chunk %d changed between runs: first=%+v second=%+v", index, first[index], second[index])
		}
	}
	if first[0].Content == "" {
		t.Fatal("first chunk content is empty")
	}
}

func TestSplitDocumentDerivesDocumentIDWhenMissing(t *testing.T) {
	document := KnowledgeDocument{Title: "MySQL", Content: "慢查询排查", Source: "runbook"}
	chunks := SplitDocument(document, 100, 10)
	if len(chunks) != 1 || chunks[0].DocID == "" || chunks[0].ChunkID == "" {
		t.Fatalf("chunks = %+v, want generated IDs", chunks)
	}
}

func TestNewIngestorRejectsInvalidWindow(t *testing.T) {
	var embedder ai.Embedder
	if _, err := NewIngestor(&ESStore{}, embedder, "mock", 100, 100); err == nil {
		t.Fatal("NewIngestor() expected dependency/window error")
	}
}
