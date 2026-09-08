package retriever

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/elastic/go-elasticsearch/v8"

	"Pilot/internal/ai"
)

// ESConfig 保存访问 Elasticsearch 所需的连接与索引配置。
// 它等价于 configutil.ESConfig，但由 retriever 包自身依赖，
// 避免覆盖层把配置结构扩散到实现包。
type ESConfig struct {
	Addresses      []string
	Username       string
	Password       string
	KnowledgeIndex string
}

// ESStore 负责索引的创建、mapping 管理以及知识片段的写入。
// 它只做「写」侧：保证索引存在、把 Chunk 写入 ES。
// 查询侧由 ESRetriever 负责。
type ESStore struct {
	client *elasticsearch.Client
	index  string
	dim    int
}

// NewESClient 创建可复用的 Elasticsearch 客户端；客户端本身不会主动发请求。
func NewESClient(cfg ESConfig) (*elasticsearch.Client, error) {
	if len(cfg.Addresses) == 0 {
		return nil, errNoESAddress
	}
	client, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: cfg.Addresses, Username: cfg.Username, Password: cfg.Password})
	if err != nil {
		return nil, fmt.Errorf("create elasticsearch client: %w", err)
	}
	return client, nil
}

// NewESStore 构造 ESStore 并确保目标索引存在（不存在则按 spec 02 创建）。
// dim 必须等于嵌入向量的维度（如 EMBEDDING_DIM），否则 mapping 与嵌入不一致。
func NewESStore(ctx context.Context, cfg ESConfig, dim int) (*ESStore, error) {
	if len(cfg.Addresses) == 0 {
		return nil, errNoESAddress
	}
	if cfg.KnowledgeIndex == "" {
		return nil, errNoESIndex
	}
	if dim <= 0 {
		return nil, errInvalidDim
	}

	client, err := NewESClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create elasticsearch client: %w", err)
	}
	store := &ESStore{client: client, index: cfg.KnowledgeIndex, dim: dim}
	if err := store.ensureIndex(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

// ensureIndex 在索引不存在时创建它，并写入 spec 02 的 mapping。
// 索引已存在时不重复创建（向量维度变更时应新建索引重建，而非改写同一索引）。
func (s *ESStore) ensureIndex(ctx context.Context) error {
	exists, err := s.client.Indices.Exists([]string{s.index}, s.client.Indices.Exists.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("check index existence: %w", err)
	}
	defer exists.Body.Close()
	// 404 表示不存在；200 表示已存在（响应 body 为空）。
	if exists.StatusCode == 200 {
		return nil
	}
	if exists.StatusCode != 404 {
		return fmt.Errorf("check index %s: status %d", s.index, exists.StatusCode)
	}

	mapping := fmt.Sprintf(`{
		"mappings": {
			"properties": {
				"doc_id":    {"type": "keyword"},
				"chunk_id":  {"type": "keyword"},
				"title":     {"type": "text", "fields": {"raw": {"type": "keyword"}}},
				"content":   {"type": "text"},
				"category":  {"type": "keyword"},
				"tags":      {"type": "keyword"},
				"source":    {"type": "keyword"},
				"version":   {"type": "integer"},
				"embedding_model": {"type": "keyword"},
				"embedding_dim":   {"type": "integer"},
				"vector": {"type": "dense_vector", "dims": %d, "index": true, "similarity": "cosine"},
				"created_at": {"type": "date"}
			}
		}
	}`, s.dim)

	//一个 index:knowledge_chunks，集合
	res, err := s.client.Indices.Create(s.index,
		s.client.Indices.Create.WithContext(ctx),
		s.client.Indices.Create.WithBody(strings.NewReader(mapping)),
	)
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	defer res.Body.Close()
	// 创建失败（如资源约束、mapping 冲突）时返回非 200。
	if res.IsError() {
		return fmt.Errorf("create index %s: status %d", s.index, res.StatusCode)
	}
	return nil
}

// esDoc 是写入 ES 的单个文档结构，字段名需与 mapping 对齐。
type esDoc struct {
	DocID          string    `json:"doc_id"`
	ChunkID        string    `json:"chunk_id"`
	Title          string    `json:"title"`
	Content        string    `json:"content"`
	Category       string    `json:"category,omitempty"`
	Tags           []string  `json:"tags,omitempty"`
	Source         string    `json:"source,omitempty"`
	Version        int       `json:"version,omitempty"`
	EmbeddingModel string    `json:"embedding_model"`
	EmbeddingDim   int       `json:"embedding_dim"`
	Vector         []float32 `json:"vector"`
	CreatedAt      string    `json:"created_at"`
}

// Index 把一个已向量化的知识片段写入索引。
// chunk.Vector 由调用方用 Embedder 生成；本方法不负责嵌入。
func (s *ESStore) Index(ctx context.Context, chunk ai.Chunk, vector []float32, embeddingModel string, createdAt string) error {
	if s == nil {
		return errNoESClient
	}
	if len(vector) != s.dim {
		return fmt.Errorf("%w: got %d, want %d", errInvalidVectorDim, len(vector), s.dim)
	}
	if s.client == nil {
		return errNoESClient
	}
	if chunk.ChunkID == "" {
		return errNoChunkID
	}
	doc := esDoc{
		DocID:          chunk.DocID,
		ChunkID:        chunk.ChunkID,
		Title:          chunk.Title,
		Content:        chunk.Content,
		Category:       chunk.Category,
		Tags:           chunk.Tags,
		Source:         chunk.Source,
		Version:        chunk.Version,
		EmbeddingModel: embeddingModel,
		EmbeddingDim:   s.dim,
		Vector:         vector,
		CreatedAt:      createdAt,
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal chunk %s: %w", chunk.ChunkID, err)
	}
	//这里是写入文档
	res, err := s.client.Index(s.index,
		bytes.NewReader(body),
		s.client.Index.WithContext(ctx),
		s.client.Index.WithDocumentID(chunk.ChunkID),
	)
	if err != nil {
		return fmt.Errorf("index chunk %s: %w", chunk.ChunkID, err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("index chunk %s: status %d", chunk.ChunkID, res.StatusCode)
	}
	return nil
}

// IndexedChunk 是已经完成 embedding 的待写入片段。
type IndexedChunk struct {
	Chunk          ai.Chunk
	Vector         []float32
	EmbeddingModel string
	CreatedAt      string
}

// IndexMany 使用 Bulk API 批量写入片段，避免每个 chunk 产生一次 HTTP 往返。
func (s *ESStore) IndexMany(ctx context.Context, chunks []IndexedChunk) error {
	if s == nil || s.client == nil {
		return errNoESClient
	}
	if len(chunks) == 0 {
		return nil
	}
	var body bytes.Buffer
	for _, item := range chunks {
		if item.Chunk.ChunkID == "" {
			return errNoChunkID
		}
		if len(item.Vector) != s.dim {
			return fmt.Errorf("%w: got %d, want %d", errInvalidVectorDim, len(item.Vector), s.dim)
		}
		doc := esDoc{
			DocID: item.Chunk.DocID, ChunkID: item.Chunk.ChunkID, Title: item.Chunk.Title,
			Content: item.Chunk.Content, Category: item.Chunk.Category, Tags: item.Chunk.Tags,
			Source: item.Chunk.Source, Version: item.Chunk.Version, EmbeddingModel: item.EmbeddingModel,
			EmbeddingDim: s.dim, Vector: item.Vector, CreatedAt: item.CreatedAt,
		}
		meta, err := json.Marshal(map[string]any{"index": map[string]string{"_id": item.Chunk.ChunkID}})
		if err != nil {
			return fmt.Errorf("marshal bulk metadata: %w", err)
		}
		data, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("marshal chunk %s: %w", item.Chunk.ChunkID, err)
		}
		body.Write(meta)
		body.WriteByte('\n')
		body.Write(data)
		body.WriteByte('\n')
	}
	res, err := s.client.Bulk(bytes.NewReader(body.Bytes()), s.client.Bulk.WithContext(ctx), s.client.Bulk.WithIndex(s.index), s.client.Bulk.WithRefresh("true"))
	if err != nil {
		return fmt.Errorf("bulk index chunks: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("bulk index chunks: status %d", res.StatusCode)
	}
	var result struct {
		Errors bool `json:"errors"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode bulk response: %w", err)
	}
	if result.Errors {
		return errors.New("bulk index chunks returned item errors")
	}
	return nil
}
