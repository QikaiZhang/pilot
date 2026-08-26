package prompt

import (
	"strings"
	"testing"

	"Pilot/internal/ai"
)

func TestBuildKnowledgeContextIncludesChunks(t *testing.T) {
	chunks := []ai.Chunk{
		{Title: "Redis 集群", Content: "Redis Cluster 提供分片与高可用", Source: "wiki", ChunkID: "c1"},
		{Title: "MySQL 主从", Content: "MySQL 主从复制用于读写分离", Source: "wiki", ChunkID: "c2"},
	}

	got := BuildKnowledgeContext(chunks)
	if !strings.Contains(got, "Redis 集群") || !strings.Contains(got, "Redis Cluster 提供分片与高可用") {
		t.Fatalf("context missing chunk content:\n%s", got)
	}
	if !strings.Contains(got, "来源：wiki") {
		t.Fatalf("context missing source annotation:\n%s", got)
	}
	if !strings.Contains(got, "[1]") || !strings.Contains(got, "[2]") {
		t.Fatalf("context missing chunk index marker:\n%s", got)
	}
}

func TestBuildKnowledgeContextEmpty(t *testing.T) {
	got := BuildKnowledgeContext(nil)
	if !strings.Contains(got, "未使用检索结果") {
		t.Fatalf("empty context should state no retrieval: %s", got)
	}
}
