package retriever

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"Pilot/internal/ai"
)

// KnowledgeDocument 是导入器接收的原始知识文档。
// 生产环境可以把它映射自 Markdown、对象存储或内部文档系统。
type KnowledgeDocument struct {
	DocID    string
	Title    string
	Content  string
	Source   string
	Category string
	Tags     []string
	Version  int
}

// Ingestor 负责文档切分、向量化和批量写入 ES。
// 它不参与查询，也不决定 Agent 是否调用 knowledge_search。
type Ingestor struct {
	store          *ESStore
	embedder       ai.Embedder
	embeddingModel string
	maxRunes       int
	overlapRunes   int
	now            func() time.Time
}

// NewIngestor 创建知识文档导入器。maxRunes 和 overlapRunes 使用字符窗口，避免引入特定 tokenizer。
func NewIngestor(store *ESStore, embedder ai.Embedder, embeddingModel string, maxRunes, overlapRunes int) (*Ingestor, error) {
	if store == nil || embedder == nil {
		return nil, ErrIngestorDependency
	}
	if maxRunes <= 0 || overlapRunes < 0 || overlapRunes >= maxRunes {
		return nil, ErrInvalidChunkWindow
	}
	return &Ingestor{store: store, embedder: embedder, embeddingModel: embeddingModel, maxRunes: maxRunes, overlapRunes: overlapRunes, now: func() time.Time { return time.Now().UTC() }}, nil
}

// Ingest 将一篇文档切成稳定片段，生成 embedding 后以 Bulk API 写入。
func (i *Ingestor) Ingest(ctx context.Context, document KnowledgeDocument) ([]ai.Chunk, error) {
	if i == nil || i.store == nil || i.embedder == nil {
		return nil, ErrIngestorDependency
	}
	if err := document.validate(); err != nil {
		return nil, err
	}
	chunks := SplitDocument(document, i.maxRunes, i.overlapRunes)
	indexed := make([]IndexedChunk, 0, len(chunks))
	createdAt := i.now().Format(time.RFC3339Nano)
	for _, chunk := range chunks {
		vector, err := i.embedder.Embed(ctx, chunk.Content)
		if err != nil {
			return nil, fmt.Errorf("embed chunk %s: %w", chunk.ChunkID, err)
		}
		indexed = append(indexed, IndexedChunk{Chunk: chunk, Vector: vector, EmbeddingModel: i.embeddingModel, CreatedAt: createdAt})
	}
	if err := i.store.IndexMany(ctx, indexed); err != nil {
		return nil, fmt.Errorf("index document %s: %w", document.DocID, err)
	}
	return chunks, nil
}

// SplitDocument 按段落优先、字符窗口兜底切分文档。
// ChunkID 只依赖文档身份、版本和序号，同一版本重复导入会覆盖原片段。
func SplitDocument(document KnowledgeDocument, maxRunes, overlapRunes int) []ai.Chunk {
	if maxRunes <= 0 || overlapRunes < 0 || overlapRunes >= maxRunes {
		return nil
	}
	docID := document.DocID
	if strings.TrimSpace(docID) == "" {
		docID = stableID(document.Source + "\x00" + document.Title)
	}
	version := document.Version
	if version <= 0 {
		version = 1
	}
	parts := splitText(strings.TrimSpace(document.Content), maxRunes, overlapRunes)
	chunks := make([]ai.Chunk, 0, len(parts))
	for index, content := range parts {
		chunks = append(chunks, ai.Chunk{
			DocID: docID, ChunkID: stableID(fmt.Sprintf("%s:%d:%d", docID, version, index)),
			Title: document.Title, Content: content, Source: document.Source,
			Category: document.Category, Tags: append([]string(nil), document.Tags...), Version: version,
		})
	}
	return chunks
}

func (d KnowledgeDocument) validate() error {
	if strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Content) == "" {
		return ErrInvalidKnowledgeDocument
	}
	return nil
}

func splitText(content string, maxRunes, overlapRunes int) []string {
	if content == "" {
		return nil
	}
	runes := []rune(content)
	result := make([]string, 0, (len(runes)+maxRunes-1)/maxRunes)
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		if end < len(runes) {
			if boundary := lastBoundary(runes[start:end]); boundary > maxRunes/2 {
				end = start + boundary
			}
		}
		result = append(result, strings.TrimSpace(string(runes[start:end])))
		if end == len(runes) {
			break
		}
		next := end - overlapRunes
		if next <= start {
			next = end
		}
		start = next
	}
	return result
}

func lastBoundary(runes []rune) int {
	for index := len(runes) - 1; index >= 0; index-- {
		if runes[index] == '\n' || runes[index] == '。' || runes[index] == '！' || runes[index] == '？' || runes[index] == '.' || runes[index] == '!' || runes[index] == '?' {
			return index + 1
		}
	}
	return 0
}

func stableID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:24]
}

var (
	ErrIngestorDependency       = errors.New("knowledge ingestor dependencies are required")
	ErrInvalidChunkWindow       = errors.New("knowledge chunk window is invalid")
	ErrInvalidKnowledgeDocument = errors.New("knowledge document title and content are required")
)
