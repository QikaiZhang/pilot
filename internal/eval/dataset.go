// Package eval 提供 RAG 检索质量评估。
// 它把评估集（JSONL）加载为 Query，对每条 query 运行 Retriever，
// 计算 HitRate@K / Recall@K / Precision@K / MRR / nDCG@K / DuplicateRate@K，
// 并按整体、category、language 输出宏平均与失败样例。
package eval

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ExpectedDoc 表示一条期望命中的文档及其可选相关性等级（0-3，越大越相关）。
// 未标注等级的样例按 relevance=1（binary）计算。
type ExpectedDoc struct {
	DocID     string  `json:"doc_id"`
	Relevance float64 `json:"relevance,omitempty"`
}

// Query 是评估集中的一条检索问题。
// 兼容两种期望写法：`expected`（带等级）与 `expected_doc_ids`（binary）。
type Query struct {
	ID       string        `json:"id"`
	Query    string        `json:"query"`
	Expected []ExpectedDoc `json:"expected,omitempty"`
	// ExpectedDocIDs 是简化写法，加载时会并入 Expected 并赋 relevance=1。
	// 它不出现在 JSON 输出里，仅用于解析兼容。
	ExpectedDocIDs    []string `json:"expected_doc_ids,omitempty"`
	Answerable        bool     `json:"answerable,omitempty"`
	RequiredFacts     []string `json:"required_facts,omitempty"`
	RequiredCitations []string `json:"required_citations,omitempty"`
	Language          string   `json:"language,omitempty"`
	Category          string   `json:"category,omitempty"`
}

// Normalize 把 ExpectedDocIDs 并入 Expected，并保证期望非空、相关性非负。
// 返回错误时，说明评估集存在不可用条目，应在上游报错而不是静默跳过。
func (q *Query) Normalize() error {
	if strings.TrimSpace(q.ID) == "" {
		return errors.New("eval query id is required")
	}
	if strings.TrimSpace(q.Query) == "" {
		return fmt.Errorf("eval query %q: query is required", q.ID)
	}
	// 兼容 expected_doc_ids 写法：并入 Expected 并按 doc_id 去重。
	merged := make(map[string]bool, len(q.Expected)+len(q.ExpectedDocIDs))
	normalized := make([]ExpectedDoc, 0, len(q.Expected)+len(q.ExpectedDocIDs))
	for _, exp := range q.Expected {
		if strings.TrimSpace(exp.DocID) == "" {
			return fmt.Errorf("eval query %q: expected doc_id is required", q.ID)
		}
		if exp.Relevance < 0 {
			return fmt.Errorf("eval query %q: relevance must be non-negative", q.ID)
		}
		if merged[exp.DocID] {
			continue
		}
		merged[exp.DocID] = true
		if exp.Relevance == 0 {
			exp.Relevance = 1
		}
		normalized = append(normalized, exp)
	}
	for _, docID := range q.ExpectedDocIDs {
		docID = strings.TrimSpace(docID)
		if docID == "" || merged[docID] {
			continue
		}
		merged[docID] = true
		normalized = append(normalized, ExpectedDoc{DocID: docID, Relevance: 1})
	}
	if len(normalized) == 0 {
		return fmt.Errorf("eval query %q: expected_doc_ids/expected required", q.ID)
	}
	q.Expected = normalized
	q.ExpectedDocIDs = nil
	return nil
}

// ExpectedSet 返回 doc_id -> relevance 的映射，供指标计算。
func (q *Query) ExpectedSet() map[string]float64 {
	set := make(map[string]float64, len(q.Expected))
	for _, exp := range q.Expected {
		set[exp.DocID] = exp.Relevance
	}
	return set
}

// LoadQueries 从 io.Reader 逐行读取 JSONL 并校验每条 Query。
// 空行会被忽略；任一行的 ID/query/期望缺失都会返回错误，避免静默污染结果。
func LoadQueries(r io.Reader) ([]Query, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var queries []Query
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var q Query
		if err := json.Unmarshal([]byte(text), &q); err != nil {
			return nil, fmt.Errorf("line %d: decode query: %w", line, err)
		}
		if err := q.Normalize(); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		queries = append(queries, q)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan queries: %w", err)
	}
	if len(queries) == 0 {
		return nil, errors.New("eval dataset is empty")
	}
	return queries, nil
}

// LoadQueriesFile 从路径加载 JSONL 评估集。
func LoadQueriesFile(path string) ([]Query, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open eval dataset %s: %w", path, err)
	}
	defer f.Close()
	return LoadQueries(f)
}
