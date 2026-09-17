// Command bench 对多代理排障编排做微基准：固定故障场景 × 固定并发，
// 用确定性脚本模型与桩工具隔离 LLM 与外部依赖，度量编排层自身的
// 耗时分布（avg/P50/P95/max）、分支超时率与降级触发率（limitations）。
//
// 指标选择的说明：端到端耗时在生产环境由模型调用主导，编排开销本身是
// 毫秒级；这里压出的价值在于超时/降级路径的行为验证与分级调度的分支
// 扇出对比，而不是模拟真实 LLM 延迟。
//
// 示例：
//
//	go run ./cmd/bench -total 100 -concurrency 20 -out reports/multiagent-bench.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	projectagent "Pilot/internal/agent"
	"Pilot/internal/multiagent"
)

type scenario struct {
	name          string
	query         string
	model         multiagent.ScriptedModel
	evidenceDelay time.Duration
	branchTimeout time.Duration // 0 表示使用编排器默认 10s
}

type scenarioReport struct {
	Name           string         `json:"name"`
	Query          string         `json:"query"`
	Total          int            `json:"total"`
	Concurrency    int            `json:"concurrency"`
	Errors         int            `json:"errors"`
	AvgMS          float64        `json:"avg_ms"`
	P50MS          float64        `json:"p50_ms"`
	P95MS          float64        `json:"p95_ms"`
	MaxMS          float64        `json:"max_ms"`
	Limitations    map[string]int `json:"limitations"`
	EvidenceCalls  int64          `json:"evidence_calls"`
	KnowledgeCalls int64          `json:"knowledge_calls"`
}

type report struct {
	GeneratedAt string           `json:"generated_at"`
	Note        string           `json:"note"`
	Scenarios   []scenarioReport `json:"scenarios"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	if err := run(); err != nil {
		slog.Error("bench failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	total := flag.Int("total", 100, "requests per scenario")
	concurrency := flag.Int("concurrency", 20, "concurrent workers per scenario")
	out := flag.String("out", "", "path to write JSON report (default: stdout)")
	flag.Parse()
	if *total <= 0 || *concurrency <= 0 {
		return fmt.Errorf("total and concurrency must be positive")
	}

	scenarios := []scenario{
		{
			// 级联信号 → complex，双分支并行：完整链路的编排开销基线。
			name:  "complex_incident_both_branches",
			query: "订单集群大面积 5xx，多个服务连环超时",
		},
		{
			// 单服务症状 → simple，只跑知识分支：分级调度省掉重分支。
			name:  "simple_incident_knowledge_only",
			query: "订单服务接口变慢",
		},
		{
			// 证据分支超过分支超时：只损失该分支（limitation），整体仍出结论。
			name:          "evidence_branch_timeout",
			query:         "订单集群大面积 5xx，多个服务连环超时",
			evidenceDelay: 300 * time.Millisecond,
			branchTimeout: 100 * time.Millisecond,
		},
		{
			// 分诊调用超过 3s 分类超时：回退规则基线，编排继续。
			name:  "classify_timeout_rule_fallback",
			query: "订单集群大面积 5xx，多个服务连环超时",
			model: multiagent.ScriptedModel{ClassifyDelay: 3500 * time.Millisecond},
		},
		{
			// 综合未引用证据编号：触发引用校验，降级为确定性摘要。
			name:  "synthesis_citation_missing",
			query: "订单集群大面积 5xx，多个服务连环超时",
			model: multiagent.ScriptedModel{SynthesisAnswer: "凭经验看是网络抖动，建议重启网关。"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	benchReport := report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Note:        "scripted model + stub tools; measures orchestration overhead, timeout rate, and degradation triggers (not LLM latency)",
	}
	for _, sc := range scenarios {
		slog.Info("running scenario", "name", sc.name, "total", *total, "concurrency", *concurrency)
		scenarioRep, err := runScenario(ctx, sc, *total, *concurrency)
		if err != nil {
			return fmt.Errorf("scenario %s: %w", sc.name, err)
		}
		benchReport.Scenarios = append(benchReport.Scenarios, scenarioRep)
	}

	data, err := json.MarshalIndent(benchReport, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if *out != "" {
		if err := os.WriteFile(*out, data, 0o644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		slog.Info("report written", "path", *out)
	} else {
		fmt.Fprintln(os.Stdout, string(data))
	}
	printSummary(benchReport)
	return nil
}

// benchTool 是可注入延迟的桩工具，calls 计数用于核对分支扇出。
type benchTool struct {
	name      string
	delay     time.Duration
	keepAlive bool
	calls     atomic.Int64
}

func (t *benchTool) Name() string                { return t.name }
func (t *benchTool) Description() string         { return "bench stub tool" }
func (t *benchTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }

func (t *benchTool) Execute(ctx context.Context, _ json.RawMessage) (any, error) {
	t.calls.Add(1)
	if t.delay > 0 {
		select {
		case <-time.After(t.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return map[string]any{"ok": true, "tool": t.name}, nil
}

func runScenario(ctx context.Context, sc scenario, total, concurrency int) (scenarioReport, error) {
	evidence := &benchTool{name: "health_check", delay: sc.evidenceDelay}
	knowledge := &benchTool{name: "knowledge_search"}
	// 降级日志在演练场景是预期输出，基准运行时静音，保持报告可读。
	orchestrator, err := multiagent.NewOrchestrator(sc.model, evidence, knowledge, nil, slog.New(slog.DiscardHandler))
	if err != nil {
		return scenarioReport{}, err
	}
	orchestrator = orchestrator.WithBranchTimeout(sc.branchTimeout)

	durations := make([]float64, 0, total)
	limitationCounts := make(map[string]int)
	var (
		mu         sync.Mutex
		errorCount int
	)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				request := projectagent.AgentRequest{
					UserID:    "bench",
					SessionID: fmt.Sprintf("bench-%d", time.Now().UnixNano()),
					Query:     sc.query,
				}
				start := time.Now()
				response, err := orchestrator.Run(ctx, request)
				elapsedMS := float64(time.Since(start)) / float64(time.Millisecond)
				mu.Lock()
				if err != nil {
					errorCount++
				} else {
					durations = append(durations, elapsedMS)
					for _, limitation := range response.Limitations {
						limitationCounts[limitation]++
					}
				}
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < total; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	if len(durations) == 0 {
		return scenarioReport{}, fmt.Errorf("all %d requests failed", total)
	}
	sort.Float64s(durations)
	return scenarioReport{
		Name:           sc.name,
		Query:          sc.query,
		Total:          total,
		Concurrency:    concurrency,
		Errors:         errorCount,
		AvgMS:          mean(durations),
		P50MS:          percentile(durations, 0.50),
		P95MS:          percentile(durations, 0.95),
		MaxMS:          durations[len(durations)-1],
		Limitations:    limitationCounts,
		EvidenceCalls:  evidence.calls.Load(),
		KnowledgeCalls: knowledge.calls.Load(),
	}, nil
}

func mean(sorted []float64) float64 {
	sum := 0.0
	for _, value := range sorted {
		sum += value
	}
	return sum / float64(len(sorted))
}

// percentile 在已排序样本上取最近邻分位数。
func percentile(sorted []float64, q float64) float64 {
	index := int(q * float64(len(sorted)-1))
	return sorted[index]
}

func printSummary(rep report) {
	fmt.Fprintf(os.Stderr, "\n== 多代理编排微基准 (generated %s) ==\n", rep.GeneratedAt)
	for _, sc := range rep.Scenarios {
		fmt.Fprintf(os.Stderr, "%-32s n=%-4d c=%-3d avg=%.2fms p50=%.2fms p95=%.2fms max=%.2fms errors=%d limitations=%v\n",
			sc.Name, sc.Total, sc.Concurrency, sc.AvgMS, sc.P50MS, sc.P95MS, sc.MaxMS, sc.Errors, sc.Limitations)
		fmt.Fprintf(os.Stderr, "%-32s evidence_calls=%d knowledge_calls=%d (fanout per request: %.2f)\n",
			"", sc.EvidenceCalls, sc.KnowledgeCalls, float64(sc.EvidenceCalls+sc.KnowledgeCalls)/float64(sc.Total))
	}
}
