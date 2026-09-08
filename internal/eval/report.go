package eval

import (
	"fmt"

	"Pilot/internal/ai"
)

// MetricSummary 是某一组结果的检索质量汇总。
type MetricSummary struct {
	N                int     `json:"n"`
	HitRateAtK       float64 `json:"hit_rate_at_k"`
	RecallAtK        float64 `json:"recall_at_k"`
	PrecisionAtK     float64 `json:"precision_at_k"`
	MRR              float64 `json:"mrr"`
	NDCGAtK          float64 `json:"ndcg_at_k"`
	DuplicateRateAtK float64 `json:"duplicate_rate_at_k"`
}

// RetrievedDoc 是报告里单条召回结果的精简视图。Rank 从 0 开始。
type RetrievedDoc struct {
	Rank    int     `json:"rank"`
	DocID   string  `json:"doc_id"`
	ChunkID string  `json:"chunk_id,omitempty"`
	Score   float64 `json:"score"`
}

// Latency 记录单次检索耗时。阶段级分解（embedding/bm25/vector/fusion）
// 需要 Retriever 提供分阶段计时，当前仅在 runner 层记录总耗时。
type Latency struct {
	EmbeddingMS float64 `json:"embedding_ms,omitempty"`
	TotalMS     float64 `json:"total_ms"`
}

// CaseResult 是单条 query 的评估记录，失败样例由此输出。
type CaseResult struct {
	ID        string         `json:"id"`
	Query     string         `json:"query"`
	Hit       bool           `json:"hit"`
	Retrieved []RetrievedDoc `json:"retrieved,omitempty"`
	Latency   Latency        `json:"latency_ms,omitempty"`
	Err       string         `json:"error,omitempty"`
}

// ReportMeta 记录一次评估的环境信息，用于复现与对比。
type ReportMeta struct {
	K           int    `json:"k"`
	Embedder    string `json:"embedder"`
	Index       string `json:"index"`
	Dataset     string `json:"dataset"`
	GeneratedAt string `json:"generated_at"`
}

// Report 写入 JSON 的完整评估结果。
type Report struct {
	ReportMeta `json:",inline"`
	Total      int                      `json:"total"`
	HitCount   int                      `json:"hit_count"`
	Errors     int                      `json:"errors"`
	Overall    MetricSummary            `json:"overall"`
	ByCategory map[string]MetricSummary `json:"by_category"`
	ByLanguage map[string]MetricSummary `json:"by_language"`
	Failures   []CaseResult             `json:"failures"`
}

// Summary 计算一组 QueryResult 的指标均值。
func Summary(results []QueryResult, k int) MetricSummary {
	return MetricSummary{
		N:                len(results),
		HitRateAtK:       HitRateAtK(results),
		RecallAtK:        RecallAtK(results),
		PrecisionAtK:     PrecisionAtK(results, k),
		MRR:              MRR(results),
		NDCGAtK:          NDCGAtK(results, k),
		DuplicateRateAtK: DuplicateRateAtK(results),
	}
}

// Evaluation 是一次评估的可读结果：原始 query、指标输入与失败样例对齐。
type Evaluation struct {
	Queries []Query
	Results []QueryResult
	Cases   []CaseResult
}

// BuildReport 聚合一次评估，并输出整体、按 category 与按 language 的宏平均。
// 分组用宏平均而非样本加总，避免某类问题数量过多掩盖其他类失效。
func BuildReport(ev Evaluation, k int, meta ReportMeta) (Report, error) {
	if len(ev.Queries) != len(ev.Results) || len(ev.Queries) != len(ev.Cases) {
		return Report{}, fmt.Errorf("evaluation slices out of sync: queries=%d results=%d cases=%d", len(ev.Queries), len(ev.Results), len(ev.Cases))
	}
	report := Report{
		ReportMeta: meta,
		Total:      len(ev.Queries),
		Overall:    Summary(ev.Results, k),
	}
	for _, c := range ev.Cases {
		if c.Err != "" {
			report.Errors++
		}
		if !c.Hit {
			report.Failures = append(report.Failures, c)
		}
	}
	report.HitCount = report.Total - len(report.Failures)
	if report.HitCount < 0 {
		report.HitCount = 0
	}

	report.ByCategory = groupSummaries(ev.Queries, ev.Results, k, func(q Query) string { return q.Category })
	report.ByLanguage = groupSummaries(ev.Queries, ev.Results, k, func(q Query) string { return q.Language })
	return report, nil
}

func groupSummaries(queries []Query, results []QueryResult, k int, key func(Query) string) map[string]MetricSummary {
	groups := make(map[string][]QueryResult)
	for i, q := range queries {
		groupKey := key(q)
		if groupKey == "" {
			continue
		}
		groups[groupKey] = append(groups[groupKey], results[i])
	}
	out := make(map[string]MetricSummary, len(groups))
	for name, subset := range groups {
		out[name] = Summary(subset, k)
	}
	return out
}

// toRetrievedDocs 把 TopK 片段转为报告视图，Rank 从 0 开始。
func toRetrievedDocs(chunks []ai.Chunk) []RetrievedDoc {
	docs := make([]RetrievedDoc, 0, len(chunks))
	for i, c := range chunks {
		docs = append(docs, RetrievedDoc{Rank: i, DocID: c.DocID, ChunkID: c.ChunkID, Score: c.Score})
	}
	return docs
}
