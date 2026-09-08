//go:build integration

package retriever

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v8"

	"Pilot/internal/ai"
	"Pilot/internal/ai/embedder"
)

// esTestEnv 读集成测试所需环境变量，缺省回退到本地 infra-up 的默认值。
// 先 make infra-up 起本地 ES（9200）再运行本测试。
func esTestEnv(t *testing.T) ESConfig {
	t.Helper()
	return ESConfig{
		Addresses:      []string{envOr("ES_URL", "http://127.0.0.1:9200")},
		Username:       os.Getenv("ES_USERNAME"),
		Password:       os.Getenv("ES_PASSWORD"),
		KnowledgeIndex: envOr("ES_KNOWLEDGE_INDEX", "pilot_knowledge_it"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestESRetrieverEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	em, err := embedder.NewMock(32)
	if err != nil {
		t.Fatal(err)
	}
	cfg := esTestEnv(t)
	cfg.KnowledgeIndex = fmt.Sprintf("pilot_knowledge_it_%d", time.Now().UnixNano())
	store, err := NewESStore(ctx, cfg, em.Dim())
	if err != nil {
		t.Fatalf("NewESStore: %v", err)
	}

	// 索引两条片段，一条关于 MySQL、一条关于 Redis。
	docs := []ai.Chunk{
		{DocID: "d1", ChunkID: "c1", Title: "MySQL 主从", Content: "MySQL 主从复制用于读写分离与容灾", Category: "数据库", Source: "wiki", Version: 1},
		{DocID: "d2", ChunkID: "c2", Title: "Redis 集群", Content: "Redis Cluster 提供分片与高可用", Category: "缓存", Source: "wiki", Version: 1},
	}
	for _, doc := range docs {
		vector, err := em.Embed(ctx, doc.Content)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Index(ctx, doc, vector, "mock", time.Now().UTC().Format(time.RFC3339)); err != nil {
			t.Fatalf("Index %s: %v", doc.ChunkID, err)
		}
	}
	// 索引是近实时的，等待刷新以确保可被检索到。
	time.Sleep(time.Second)

	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: cfg.Addresses, Username: cfg.Username, Password: cfg.Password})
	if err != nil {
		t.Fatal(err)
	}
	retriever, err := NewESRetriever(client, cfg.KnowledgeIndex, em, 3, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// 查「MySQL」，预期召回以 MySQL 片段居首（关键词与向量都指向它）。
	chunks, err := retriever.Retrieve(ctx, "MySQL 主从复制怎么做", 3)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("Retrieve returned no chunks")
	}
	if chunks[0].ChunkID != "c1" {
		t.Fatalf("top chunk = %q (content %q), want c1 (MySQL 片段)", chunks[0].ChunkID, chunks[0].Content)
	}

	// 空查询应作为错误返回，而不是静默全量召回。
	if _, err := retriever.Retrieve(ctx, "   ", 3); err == nil {
		t.Fatal("Retrieve with empty query expected error")
	}
}

func TestKnowledgeIngestorBulkThenRetrieve(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	em, err := embedder.NewMock(32)
	if err != nil {
		t.Fatal(err)
	}
	cfg := esTestEnv(t)
	cfg.KnowledgeIndex = fmt.Sprintf("pilot_knowledge_ingest_it_%d", time.Now().UnixNano())
	store, err := NewESStore(ctx, cfg, em.Dim())
	if err != nil {
		t.Fatal(err)
	}
	ingestor, err := NewIngestor(store, em, "mock", 40, 8)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := ingestor.Ingest(ctx, KnowledgeDocument{
		DocID: "redis-runbook", Title: "Redis 超时排查", Content: "客户端连接超时。检查连接池、网络和 Redis 负载。", Source: "testdata/rag", Category: "缓存", Version: 1,
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(chunks) == 0 || chunks[0].ChunkID == "" {
		t.Fatalf("chunks = %+v", chunks)
	}

	client, err := NewESClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	retriever, err := NewESRetriever(client, cfg.KnowledgeIndex, em, 3, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	results, err := retriever.Retrieve(ctx, "Redis 连接超时", 3)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(results) == 0 || results[0].DocID != "redis-runbook" {
		t.Fatalf("results = %+v, want redis-runbook", results)
	}
}
