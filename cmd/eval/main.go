// Command eval 运行 RAG 检索质量评估。
// 它从 JSONL 加载评估集，对每条 query 调用 ESRetriever，
// 计算 HitRate@K / Recall@K / Precision@K / MRR / nDCG@K / DuplicateRate@K，
// 并把整体、按 category/language 的宏平均与失败样例写入 JSON 报告。
//
// 示例：
//
//	# 纯 RRF 基线
//	go run ./cmd/eval -dataset testdata/rag/eval/queries.jsonl -k 5 -out reports/rag-eval.json
//	# 评估前重新导入语料（保证评估可复现）
//	go run ./cmd/eval -import testdata/rag/corpus -out reports/rag-eval.json
//	# LLM listwise 精排对比实验（需要 LLM_MODE=eino|openai 与 LLM_API_KEY）
//	go run ./cmd/eval -import testdata/rag/corpus -rerank -out reports/rag-eval-rerank.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"Pilot/internal/ai"
	einoadapter "Pilot/internal/ai/eino"
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
	importDir := flag.String("import", "", "ingest a corpus directory (*.md) into ES before evaluating, e.g. testdata/rag/corpus")
	rerank := flag.Bool("rerank", false, "wrap retrieval with LLM listwise rerank (requires LLM_MODE=eino|openai)")
	flag.Parse()

	_ = envloader.Load(".env")
	cfg := configutil.Load()

	queries, err := eval.LoadQueriesFile(*ds)
	if err != nil {
		return fmt.Errorf("load dataset: %w", err)
	}
	slog.Info("eval dataset loaded", "path", *ds, "queries", len(queries))

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(len(queries)+10)*time.Minute)
	defer cancel()

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
	if *importDir != "" {
		docs, chunks, err := importCorpus(ctx, esConfig, em, *importDir)
		if err != nil {
			return fmt.Errorf("import corpus: %w", err)
		}
		slog.Info("corpus imported", "dir", *importDir, "documents", docs, "chunks", chunks)
	}

	// 精排作为装饰器接入评估链路：评估器只看 ai.Retriever 接口，
	// 是否精排对指标计算完全透明。
	var evalRetriever ai.Retriever = searcher
	if *rerank {
		chatModel, err := newEvalChatModel(ctx, cfg)
		if err != nil {
			return err
		}
		evalRetriever = mustDecorate(searcher, chatModel)
		slog.Info("rerank enabled", "mode", "llm_listwise")
	}

	runner, err := eval.NewRunner(evalRetriever, *timeout)
	if err != nil {
		return err
	}
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

// importCorpus 把目录下的 Markdown 文档导入 ES 索引，保证评估可复现：
// docID 取文件名去扩展名（必须与评估集 expected 的 doc_id 严格一致），
// 分类按文件名前缀映射，标题取首个一级标题，缺省用文件名。
func importCorpus(ctx context.Context, esConfig retriever.ESConfig, em *embedder.MockEmbedder, dir string) (int, int, error) {
	store, err := retriever.NewESStore(ctx, esConfig, em.Dim())
	if err != nil {
		return 0, 0, fmt.Errorf("create es store: %w", err)
	}
	ingestor, err := retriever.NewIngestor(store, em, "mock", 1800, 200)
	if err != nil {
		return 0, 0, fmt.Errorf("create ingestor: %w", err)
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return 0, 0, err
	}
	sort.Strings(paths)
	docs, chunks := 0, 0
	for _, path := range paths {
		docID := strings.TrimSuffix(filepath.Base(path), ".md")
		content, err := os.ReadFile(path)
		if err != nil {
			return docs, chunks, err
		}
		inserted, err := ingestor.Ingest(ctx, retriever.KnowledgeDocument{
			DocID:    docID,
			Title:    firstHeading(string(content), docID),
			Content:  string(content),
			Category: categoryForDocID(docID),
			Version:  1,
			Source:   path,
		})
		if err != nil {
			return docs, chunks, fmt.Errorf("ingest %s: %w", docID, err)
		}
		docs++
		chunks += len(inserted)
	}
	return docs, chunks, nil
}

// firstHeading 提取 Markdown 首个一级标题作为文档标题。
func firstHeading(content, fallback string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return fallback
}

// categoryForDocID 按文件名前缀映射分类，与评估集的 category 保持一致。
func categoryForDocID(docID string) string {
	switch {
	case strings.HasPrefix(docID, "redis"):
		return "cache"
	case strings.HasPrefix(docID, "mysql"), strings.HasPrefix(docID, "es"), strings.HasPrefix(docID, "elasticsearch"):
		return "database"
	default:
		return "general"
	}
}

// newEvalChatModel 为精排构造真实模型客户端。mock 模式直接报错：
// 避免"看起来在评估精排、实际全是 RRF 基线"的假对比。
func newEvalChatModel(ctx context.Context, cfg configutil.Config) (ai.ChatModel, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.LLM.Mode)) {
	case "eino", "openai":
		return einoadapter.NewChatModel(ctx, einoadapter.Config{
			APIKey:      cfg.LLM.APIKey,
			BaseURL:     cfg.LLM.BaseURL,
			Model:       cfg.LLM.Model,
			Temperature: cfg.LLM.Temperature,
			MaxTokens:   cfg.LLM.MaxTokens,
			Timeout:     cfg.LLM.Timeout,
		})
	default:
		return nil, fmt.Errorf("rerank requires a real LLM: set LLM_MODE=eino|openai with LLM_API_KEY (current mode %q)", cfg.LLM.Mode)
	}
}

func mustDecorate(searcher ai.Retriever, chatModel ai.ChatModel) ai.Retriever {
	decorated, err := retriever.NewRerankedRetriever(searcher, retriever.NewLLMReranker(chatModel, nil), retriever.RerankConfig{})
	if err != nil {
		// 参数全部来自已校验的构造产物，这里失败只可能是编程错误。
		panic(fmt.Sprintf("decorate retriever with reranker: %v", err))
	}
	return decorated
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
