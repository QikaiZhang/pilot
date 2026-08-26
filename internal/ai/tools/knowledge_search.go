// Package tools 定义 Agent 可调用的外部工具。
// 每个工具实现 Tool 接口，接收结构化参数、返回结构化结果；
// Agent 只负责决策调用，工具负责访问外部系统（这里是知识库检索）。
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"Pilot/internal/ai"
)

// Tool 描述一个可被模型/Agen调用并返回结构化结果的外部能力。
// 入参出参均为 JSON 可序列化对象，便于模型理解与后续接入 Eino ReAct。
type Tool interface {
	// Name 返回工具名，供模型引用。
	Name() string
	// Execute 执行工具并返回 JSON 序列化的结果。
	Execute(ctx context.Context, input json.RawMessage) (any, error)
}

// KnowledgeSearchInput 是知识检索工具的入参。
type KnowledgeSearchInput struct {
	Query    string `json:"query"`
	TopK     int    `json:"top_k,omitempty"`
	Category string `json:"category,omitempty"`
}

// KnowledgeSearchResult 是知识检索工具的出参。
// 结构化返回命中的片段，供 Agent 注入上下文或生成依据。
type KnowledgeSearchResult struct {
	Results  []ai.Chunk `json:"results"`
	Query    string     `json:"query"`
	HitCount int        `json:"hit_count"`
}

// KnowledgeSearch 封装 ai.Retriever，供 Agent 作为「知识库检索」工具调用。
type KnowledgeSearch struct {
	retriever ai.Retriever
}

// NewKnowledgeSearch 构造知识检索工具。retriever 负责底层混合召回。
func NewKnowledgeSearch(retriever ai.Retriever) *KnowledgeSearch {
	return &KnowledgeSearch{retriever: retriever}
}

// Name 返回工具名。
func (k *KnowledgeSearch) Name() string {
	return "knowledge_search"
}

// Execute 解析入参、执行检索并返回结构化结果。
// 空 query 或未配置 retriever 时返回可读错误，不泄露底层细节。
func (k *KnowledgeSearch) Execute(ctx context.Context, input json.RawMessage) (any, error) {
	if k == nil || k.retriever == nil {
		return nil, errRetrieverUnavailable
	}
	var params KnowledgeSearchInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, errInvalidInput
	}
	if params.Query == "" {
		return nil, errEmptyQuery
	}
	topK := params.TopK
	if topK <= 0 {
		topK = defaultTopK
	}
	if params.Category != "" {
		// 分类过滤是精确 keyword 匹配；检索后过滤，同时保留混合分数排序。
		return k.searchWithFilter(ctx, params.Query, topK, params.Category)
	}
	return k.search(ctx, params.Query, topK)
}

func (k *KnowledgeSearch) search(ctx context.Context, query string, topK int) (any, error) {
	chunks, err := k.retriever.Retrieve(ctx, query, topK)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errRetrieve, err)
	}
	return KnowledgeSearchResult{Results: chunks, Query: query, HitCount: len(chunks)}, nil
}

func (k *KnowledgeSearch) searchWithFilter(ctx context.Context, query string, topK int, category string) (any, error) {
	// 多取一些再过滤，保证过滤后仍可能凑够 topK。
	chunks, err := k.retriever.Retrieve(ctx, query, filterBuffer(topK))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errRetrieve, err)
	}
	filtered := make([]ai.Chunk, 0, topK)
	for _, chunk := range chunks {
		if chunk.Category == category {
			filtered = append(filtered, chunk)
		}
		if len(filtered) >= topK {
			break
		}
	}
	return KnowledgeSearchResult{Results: filtered, Query: query, HitCount: len(filtered)}, nil
}

const (
	defaultTopK = 5
	filterScale = 3 // 过滤前多取倍数，避免过滤后不足 topK
)

func filterBuffer(topK int) int {
	return topK * filterScale
}
