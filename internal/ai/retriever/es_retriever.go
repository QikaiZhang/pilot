package retriever

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"

	"Pilot/internal/ai"
)

// ESRetriever 实现 ai.Retriever：对 query 同时做关键词（BM25）与向量（kNN）
// 两路召回，经 RRF 融合后按相关度降序返回。
type ESRetriever struct {
	client   *elasticsearch.Client
	index    string
	embedder ai.Embedder
	topK     int
	timeout  time.Duration
}

// searchResponse 对应 ES /_search 响应的 hits 结构。
type searchResponse struct {
	Hits struct {
		Hits []escHit `json:"hits"`
	} `json:"hits"`
}

// escHit 是单条命中结果，_source 为原始文档，_score 为来源分数。
type escHit struct {
	ID     string   `json:"_id"`
	Score  float64  `json:"_score"`
	Source esSource `json:"_source"`
}

// esSource 对应写入索引的文档字段（其中 content 用于返回，其余用于过滤/追溯）。
type esSource struct {
	DocID    string `json:"doc_id"`
	ChunkID  string `json:"chunk_id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Category string `json:"category"`
	Source   string `json:"source"`
	Version  int    `json:"version"`
}

func (h escHit) toChunk() ai.Chunk {
	return ai.Chunk{
		DocID:    h.Source.DocID,
		ChunkID:  h.Source.ChunkID,
		Title:    h.Source.Title,
		Content:  h.Source.Content,
		Category: h.Source.Category,
		Source:   h.Source.Source,
		Version:  h.Source.Version,
		Score:    h.Score,
	}
}

// NewESRetriever 构造混合检索器。embedder 用于把 query 编码成查询向量，
// 必须与索引侧写入向量时所用的 Embedder 一致（同模型、同维度）。
func NewESRetriever(client *elasticsearch.Client, index string, embedder ai.Embedder, topK int, timeout time.Duration) (*ESRetriever, error) {
	if client == nil {
		return nil, errNoESClient
	}
	if index == "" {
		return nil, errNoESIndex
	}
	if embedder == nil {
		return nil, errNoEmbedder
	}
	if topK <= 0 {
		return nil, errInvalidTopK
	}
	return &ESRetriever{
		client:   client,
		index:    index,
		embedder: embedder,
		topK:     topK,
		timeout:  timeout,
	}, nil
}

// Retrieve 执行混合召回。返回的片段按融合分降序；空召回返回空切片而非错误。
func (r *ESRetriever) Retrieve(ctx context.Context, query string, topK int) ([]ai.Chunk, error) {
	if r == nil || r.client == nil {
		return nil, errNoESClient
	}
	if strings.TrimSpace(query) == "" {
		return nil, errEmptyQuery
	}
	if topK <= 0 {
		return nil, errInvalidTopK
	}

	// 向量化 query；索引侧与查询侧必须同一 Embedder。
	vector, err := r.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	bm25Hits, err := r.searchBM25(ctx, query, topK)
	if err != nil {
		return nil, err
	}
	vectorHits, err := r.searchKNN(ctx, vector, topK)
	if err != nil {
		return nil, err
	}

	return fuseRRF(bm25Hits, vectorHits), nil
}

// searchBM25 用全文 match 对 content 做关键词召回。
func (r *ESRetriever) searchBM25(ctx context.Context, query string, size int) ([]ai.Chunk, error) {
	body, err := marshalSearch(map[string]any{
		"size": size,
		"query": map[string]any{
			"bool": map[string]any{
				"must": []any{
					map[string]any{"match": map[string]any{"content": query}},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return r.doSearch(ctx, body)
}

// searchKNN 用 dense_vector 的 kNN 检索做语义召回。
// kNN 检索要求维度与索引一致；query_vector 用 Embedder 生成。
func (r *ESRetriever) searchKNN(ctx context.Context, vector []float32, size int) ([]ai.Chunk, error) {
	body, err := marshalSearch(map[string]any{
		"knn": map[string]any{
			"field":          "vector",
			"query_vector":   vector,
			"k":              size,
			"num_candidates": size * 2,
		},
		"size": size,
	})
	if err != nil {
		return nil, err
	}
	return r.doSearch(ctx, body)
}

// marshalSearch 把查询结构编码为 ES /_search 请求体。
func marshalSearch(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal search body: %w", err)
	}
	return string(data), nil
}

// doSearch 发送搜索请求并解析 hits，把 ES 文档映射为业务 Chunk。
func (r *ESRetriever) doSearch(ctx context.Context, body string) ([]ai.Chunk, error) {
	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex(r.index),
		r.client.Search.WithBody(strings.NewReader(body)),
		r.client.Search.WithTimeout(r.timeout),
	)
	if err != nil {
		return nil, fmt.Errorf("search index %s: %w", r.index, err)
	}
	defer res.Body.Close()
	if res.IsError() {
		// 只报告状态码，不把 ES 原始错误文本带进业务层。
		return nil, fmt.Errorf("search index %s: status %d", r.index, res.StatusCode)
	}

	var sr searchResponse
	if err := json.NewDecoder(res.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	chunks := make([]ai.Chunk, 0, len(sr.Hits.Hits))
	for _, hit := range sr.Hits.Hits {
		chunks = append(chunks, hit.toChunk())
	}
	return chunks, nil
}
