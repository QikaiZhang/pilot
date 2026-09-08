package eval

import (
	"context"
	"errors"
	"time"

	"Pilot/internal/ai"
)

// Runner 运行一次检索评估：对每条 query 调用 Retriever，收集召回与耗时，
// 并产出指标输入（QueryResult）与失败样例（CaseResult）。
type Runner struct {
	Retriever ai.Retriever
	// Timeout 是单条 query 检索的超时；<=0 时使用调用方 context 不额外限制。
	Timeout time.Duration
}

// NewRunner 构造评估运行器。Retriever 为 nil 时返回错误，避免最终指标全为空。
func NewRunner(retriever ai.Retriever, timeout time.Duration) (*Runner, error) {
	if retriever == nil {
		return nil, ErrRunnerDependency
	}
	return &Runner{Retriever: retriever, Timeout: timeout}, nil
}

// ErrRunnerDependency 表示评估运行器缺少 Retriever 依赖。
var ErrRunnerDependency = errors.New("eval runner retriever is required")

// ErrInvalidK 表示评估的 Top-K 必须为正数。
var ErrInvalidK = errors.New("eval top-k must be positive")

// Evaluate 对每个 query 运行一次检索，返回与 queries 对齐的评估结果。
// 任一 query 检索出错时记录到对应 Case，并把该 query 的指标输入置为空召回，
// 使检索失败作为「未命中」计入，避免指标分母漂移。
func (r *Runner) Evaluate(ctx context.Context, queries []Query, k int) (Evaluation, error) {
	if k <= 0 {
		return Evaluation{}, ErrInvalidK
	}
	if len(queries) == 0 {
		return Evaluation{}, nil
	}
	ev := Evaluation{Queries: queries, Results: make([]QueryResult, 0, len(queries)), Cases: make([]CaseResult, 0, len(queries))}
	for _, q := range queries {
		qr := QueryResult{ID: q.ID, Expected: q.ExpectedSet(), DuplicateRatio: -1}
		cs := CaseResult{ID: q.ID, Query: q.Query}

		retrieveCtx := ctx
		cancel := func() {}
		if r.Timeout > 0 {
			retrieveCtx, cancel = context.WithTimeout(ctx, r.Timeout)
		}
		chunks, elapsed, err := r.retrieve(retrieveCtx, q.Query, k)
		cancel()

		cs.Latency = Latency{TotalMS: elapsed}
		qr.DuplicateRatio = DuplicateRatio(chunks, k)
		if err != nil {
			cs.Err = err.Error()
			ev.Cases = append(ev.Cases, cs)
			ev.Results = append(ev.Results, qr)
			continue
		}

		qr.Retrieved = UnpackDocIDs(chunks, k)
		cs.Retrieved = toRetrievedDocs(chunks)
		cs.Hit = isHit(qr.Expected, qr.Retrieved)
		ev.Cases = append(ev.Cases, cs)
		ev.Results = append(ev.Results, qr)
	}
	return ev, nil
}

// retrieve 执行一次检索并记录总耗时（毫秒）。
func (r *Runner) retrieve(ctx context.Context, query string, k int) ([]ai.Chunk, float64, error) {
	start := time.Now()
	chunks, err := r.Retriever.Retrieve(ctx, query, k)
	return chunks, float64(time.Since(start).Milliseconds()), err
}

// isHit 判断召回的 doc 列表是否命中至少一个期望文档。
func isHit(expected map[string]float64, retrieved []string) bool {
	for _, docID := range retrieved {
		if _, ok := expected[docID]; ok {
			return true
		}
	}
	return false
}
