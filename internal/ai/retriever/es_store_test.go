package retriever

import (
	"strings"
	"testing"

	"Pilot/internal/ai"
)

func TestESStoreIndexRejectsVectorDimensionMismatch(t *testing.T) {
	store := &ESStore{dim: 32}
	err := store.Index(nil, ai.Chunk{ChunkID: "c1"}, make([]float32, 31), "mock", "2026-01-01T00:00:00Z")
	if err == nil || !strings.Contains(err.Error(), "embedding vector dimension does not match index") {
		t.Fatalf("Index() error = %v, want vector dimension error", err)
	}
}
