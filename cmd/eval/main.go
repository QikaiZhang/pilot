// Command eval 运行 RAG 检索质量评估。
// 它从 JSONL 加载评估集，对每条 query 调用 ESRetriever，
// 计算 HitRate@K / Recall@K / Precision@K / MRR / nDCG@K / DuplicateRate@K，
// 并把整体、按 category/language 的宏平均与失败样例写入 JSON 报告。
//
// 示例：
//
//	go run ./cmd/eval -dataset testdata/rag/eval/queries.jsonl -k 5 -out reports/rag-eval.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"Pilot/internal/ai/embedder"
	"Pilot/internal/ai/retriever"
	"Pilot/internal/eval"
	"Pilot/pkg/configutil"
	"Pilot/pkg/envloader"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(); err != nil {
		slog.Error("rag eval failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ds := flag.String("dataset", "testdata/rag/eval/queries.jsonl", "path to JSONL eval dataset")
	out := flag.String("out", "", "path to write JSON report (default: stdout)")
	k := flag.Int("k", 5, "top-K for retrieval")
	timeout := flag.Duration("timeout", 5*time.Second, "per-query retrieval timeout")
	flag.Parse()

	_ = envloader.Load(".env")
	cfg := configutil.Load()

	queries, err := eval.LoadQueriesFile(*ds)
	if err != nil {
		return fmt.Errorf("load dataset: %w", err)
	}
	slog.Info("eval dataset loaded", "path", *ds, "queries", len(queries))

	esConfig := retriever.ESConfig{
		Addresses:      []string{cfg.ES.URL},
		Username:       cfg.ES.Username,
		Password:       cfg.ES.Password,
		KnowledgeIndex: cfg.ES.KnowledgeIndex,
	}
	if cfg.Embedding.Mode != "mock" {
		return fmt.Errorf("embedding mode %q is not supported by cmd/eval; evaluation must use the same embedder as ingestion", cfg.Embedding.Mode)
	}
	em, err := embedder.NewMock(cfg.Embedding.Dim)
	if err != nil {
		return fmt.Errorf("create embedder: %w", err)
	}
	client, err := retriever.NewESClient(esConfig)
	if err != nil {
		return fmt.Errorf("create es client: %w", err)
	}
	searcher, err := retriever.NewESRetriever(client, cfg.ES.KnowledgeIndex, em, *k, *timeout)
	if err != nil {
		return fmt.Errorf("create retriever: %w", err)
	}

	runner, err := eval.NewRunner(searcher, *timeout)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(len(queries)+10)*time.Minute)
	defer cancel()
	ev, err := runner.Evaluate(ctx, queries, *k)
	if err != nil {
		return fmt.Errorf("evaluate: %w", err)
	}

	report, err := eval.BuildReport(ev, *k, eval.ReportMeta{
		K:           *k,
		Embedder:    "mock",
		Index:       cfg.ES.KnowledgeIndex,
		Dataset:     *ds,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("build report: %w", err)
	}

	reportJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if *out != "" {
		if err := os.WriteFile(*out, reportJSON, 0o644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		slog.Info("report written", "path", *out)
	} else {
		fmt.Fprintln(os.Stdout, string(reportJSON))
	}
	printSummary(report)
	return nil
}

// printSummary 把摘要写到 stderr，保持 stdout 只输出机器可读 JSON。
func printSummary(report eval.Report) {
	fmt.Fprintf(os.Stderr, "\n== RAG 评估摘要 (k=%d, n=%d) ==\n", report.K, report.Total)
	fmt.Fprintf(os.Stderr, "hit=%.1f%% recall=%.1f%% precision=%.1f%% mrr=%.3f ndcg=%.3f dup=%.1f%% errors=%d\n",
		report.Overall.HitRateAtK*100, report.Overall.RecallAtK*100, report.Overall.PrecisionAtK*100,
		report.Overall.MRR, report.Overall.NDCGAtK, report.Overall.DuplicateRateAtK*100, report.Errors)
	for name, s := range report.ByCategory {
		fmt.Fprintf(os.Stderr, "category %-12s n=%-3d hit=%.1f%% recall=%.1f%% mrr=%.3f\n", name, s.N, s.HitRateAtK*100, s.RecallAtK*100, s.MRR)
	}
	if len(report.Failures) > 0 {
		fmt.Fprintf(os.Stderr, "\n失败样例 (%d):\n", len(report.Failures))
		for _, f := range report.Failures {
			fmt.Fprintf(os.Stderr, "  - %s (%s) err=%s\n", f.ID, f.Query, f.Err)
		}
	}
}
