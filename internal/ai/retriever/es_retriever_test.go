package retriever

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarshalSearchEncodesMatchQuery(t *testing.T) {
	body, err := marshalSearch(map[string]any{
		"size": 5,
		"query": map[string]any{
			"bool": map[string]any{
				"must": []any{map[string]any{"match": map[string]any{"content": `含有 "引号" 的查询`}}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(body)) {
		t.Fatalf("body is not valid JSON: %s", body)
	}
	if !strings.Contains(body, "match") || !strings.Contains(body, `\"引号\"`) {
		t.Fatalf("body = %s, want match query with escaped quotes", body)
	}
}

func TestEscHitToChunk(t *testing.T) {
	hit := escHit{
		ID:    "chunk-1",
		Score: 2.5,
		Source: esSource{
			DocID:    "doc-1",
			ChunkID:  "chunk-1",
			Title:    "标题",
			Content:  "正文",
			Category: "故障",
			Source:   "wiki",
			Version:  3,
		},
	}

	chunk := hit.toChunk()
	if chunk.DocID != "doc-1" || chunk.ChunkID != "chunk-1" || chunk.Title != "标题" || chunk.Content != "正文" {
		t.Fatalf("toChunk() = %+v", chunk)
	}
	if chunk.Category != "故障" || chunk.Source != "wiki" || chunk.Version != 3 {
		t.Fatalf("toChunk() metadata = %+v", chunk)
	}
	if chunk.Score != 2.5 {
		t.Fatalf("toChunk() score = %v, want 2.5", chunk.Score)
	}
}

func TestSearchResponseDecoding(t *testing.T) {
	raw := `{
		"hits": {
			"hits": [
				{"_id": "c1", "_score": 1.2, "_source": {"chunk_id": "c1", "content": "第一段"}},
				{"_id": "c2", "_score": 0.9, "_source": {"chunk_id": "c2", "content": "第二段"}}
			]
		}
	}`
	var sr searchResponse
	if err := json.Unmarshal([]byte(raw), &sr); err != nil {
		t.Fatal(err)
	}
	if len(sr.Hits.Hits) != 2 {
		t.Fatalf("decoded hits = %d, want 2", len(sr.Hits.Hits))
	}
	if sr.Hits.Hits[0].toChunk().ChunkID != "c1" {
		t.Fatalf("first chunk = %+v", sr.Hits.Hits[0].toChunk())
	}
}
