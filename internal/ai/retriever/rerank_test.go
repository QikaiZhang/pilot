package retriever

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"Pilot/internal/ai"
)

type rerankFakeInner struct {
	gotTopK []int
	batches [][]ai.Chunk
}

func (f *rerankFakeInner) Retrieve(_ context.Context, _ string, topK int) ([]ai.Chunk, error) {
	f.gotTopK = append(f.gotTopK, topK)
	next := f.batches[0]
	f.batches = f.batches[1:]
	return next, nil
}

type rerankScripted struct {
	calls int
	run   func(ctx context.Context, query string, candidates []ai.Chunk) ([]ai.Chunk, error)
}

func (s *rerankScripted) Rerank(ctx context.Context, query string, candidates []ai.Chunk) ([]ai.Chunk, error) {
	s.calls++
	return s.run(ctx, query, candidates)
}

func rerankChunk(docID string) ai.Chunk {
	return ai.Chunk{DocID: docID, ChunkID: docID + "-c1", Title: "t-" + docID, Content: "content " + docID}
}

func TestRerankedRetrieverOverfetchesReordersAndCuts(t *testing.T) {
	inner := &rerankFakeInner{batches: [][]ai.Chunk{{
		rerankChunk("a"), rerankChunk("b"), rerankChunk("c"), rerankChunk("d"),
	}}}
	reranker := &rerankScripted{run: func(_ context.Context, _ string, candidates []ai.Chunk) ([]ai.Chunk, error) {
		return []ai.Chunk{candidates[2], candidates[0], candidates[1], candidates[3]}, nil
	}}
	decorated, err := NewRerankedRetriever(inner, reranker, RerankConfig{Candidates: 20, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}

	chunks, err := decorated.Retrieve(context.Background(), "q", 2)
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(inner.gotTopK) != 1 || inner.gotTopK[0] != 20 {
		t.Fatalf("inner gotTopK = %v, want over-fetch of 20", inner.gotTopK)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks = %v, want cut to topK", chunkDocIDs(chunks))
	}
	want := []string{"c", "a"}
	if !slices.Equal(chunkDocIDs(chunks), want) {
		t.Fatalf("order = %v, want %v", chunkDocIDs(chunks), want)
	}
}

func TestRerankedRetrieverDegradesToRecallOrderOnError(t *testing.T) {
	inner := &rerankFakeInner{batches: [][]ai.Chunk{{
		rerankChunk("a"), rerankChunk("b"), rerankChunk("c"),
	}}}
	reranker := &rerankScripted{run: func(context.Context, string, []ai.Chunk) ([]ai.Chunk, error) {
		return nil, errors.New("llm unavailable")
	}}
	decorated, err := NewRerankedRetriever(inner, reranker, RerankConfig{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}

	chunks, err := decorated.Retrieve(context.Background(), "q", 2)
	if err != nil {
		// 降级语义：精排失败不改变业务语义，不向调用方上抛。
		t.Fatalf("Retrieve() error = %v, want degraded success", err)
	}
	want := []string{"a", "b"}
	if !slices.Equal(chunkDocIDs(chunks), want) {
		t.Fatalf("order = %v, want recall order %v", chunkDocIDs(chunks), want)
	}
}

func TestRerankedRetrieverDegradesOnRerankTimeout(t *testing.T) {
	inner := &rerankFakeInner{batches: [][]ai.Chunk{{
		rerankChunk("a"), rerankChunk("b"),
	}}}
	reranker := &rerankScripted{run: func(ctx context.Context, _ string, _ []ai.Chunk) ([]ai.Chunk, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	decorated, err := NewRerankedRetriever(inner, reranker, RerankConfig{Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}

	chunks, err := decorated.Retrieve(context.Background(), "q", 5)
	if err != nil {
		t.Fatalf("Retrieve() error = %v, want degraded success", err)
	}
	want := []string{"a", "b"}
	if !slices.Equal(chunkDocIDs(chunks), want) {
		t.Fatalf("order = %v, want recall order %v", chunkDocIDs(chunks), want)
	}
}

func TestRerankedRetrieverSkipsRerankWhenCandidatesInsufficient(t *testing.T) {
	inner := &rerankFakeInner{batches: [][]ai.Chunk{{rerankChunk("a")}}}
	reranker := &rerankScripted{run: func(context.Context, string, []ai.Chunk) ([]ai.Chunk, error) {
		t.Fatal("rerank should not run for a single candidate")
		return nil, nil
	}}
	decorated, err := NewRerankedRetriever(inner, reranker, RerankConfig{})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := decorated.Retrieve(context.Background(), "q", 5)
	if err != nil || len(chunks) != 1 {
		t.Fatalf("chunks = %v, err = %v", chunkDocIDs(chunks), err)
	}
}

func TestRerankedRetrieverFetchesAtLeastTopK(t *testing.T) {
	inner := &rerankFakeInner{batches: [][]ai.Chunk{{
		rerankChunk("a"), rerankChunk("b"), rerankChunk("c"),
	}}}
	reranker := &rerankScripted{run: func(_ context.Context, _ string, candidates []ai.Chunk) ([]ai.Chunk, error) {
		return candidates, nil
	}}
	decorated, err := NewRerankedRetriever(inner, reranker, RerankConfig{Candidates: 20})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decorated.Retrieve(context.Background(), "q", 30); err != nil {
		t.Fatal(err)
	}
	if inner.gotTopK[0] != 30 {
		t.Fatalf("inner gotTopK = %v, want 30 (topK wins over default candidates)", inner.gotTopK)
	}
}

func TestRerankedRetrieverFillsDefaults(t *testing.T) {
	decorated, err := NewRerankedRetriever(
		&rerankFakeInner{batches: [][]ai.Chunk{nil}},
		&rerankScripted{}, RerankConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if decorated.cfg.Candidates != defaultRerankCandidates {
		t.Fatalf("candidates = %d, want %d", decorated.cfg.Candidates, defaultRerankCandidates)
	}
	if decorated.cfg.Timeout != defaultRerankTimeout {
		t.Fatalf("timeout = %v, want %v", decorated.cfg.Timeout, defaultRerankTimeout)
	}
	if _, err := NewRerankedRetriever(nil, &rerankScripted{}, RerankConfig{}); !errors.Is(err, errRerankDependency) {
		t.Fatalf("missing inner error = %v", err)
	}
}

func TestParseRanking(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		n        int
		want     []int
		complete bool
		ok       bool
	}{
		{"partial prefix", "3 1 7", 10, []int{3, 1, 7}, false, true},
		{"full permutation", "10 9 8 7 6 5 4 3 2 1", 10, []int{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}, true, true},
		{"duplicate dropped", "1 1 2", 3, []int{1, 2}, false, true},
		{"out of range dropped", "0 5 12 3", 4, []int{3}, false, true},
		{"garbage tokens skipped", "abc 2 xyz 1", 3, []int{2, 1}, false, true},
		{"think block stripped", "<think>candidates 9 9 9</think>\n2 1", 3, []int{2, 1}, false, true},
		{"code fence stripped", "```\n3 1\n```", 5, []int{3, 1}, false, true},
		{"enumeration markers", "1. 3. 2.", 3, []int{1, 3, 2}, true, true},
		{"no valid numbers", "我觉得都不错", 3, nil, false, false},
		{"empty", "", 3, nil, false, false},
		{"unclosed think abandoned", "<think>maybe 2 1", 3, nil, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			order, complete, ok := parseRanking(tc.content, tc.n)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !slices.Equal(order, tc.want) {
				t.Fatalf("order = %v, want %v", order, tc.want)
			}
			if complete != tc.complete {
				t.Fatalf("complete = %v, want %v", complete, tc.complete)
			}
		})
	}
}

func TestApplyRankingBackfillsInOriginalOrder(t *testing.T) {
	candidates := []ai.Chunk{rerankChunk("a"), rerankChunk("b"), rerankChunk("c"), rerankChunk("d")}
	ranked := applyRanking(candidates, []int{3, 1})
	want := []string{"c", "a", "b", "d"}
	if !slices.Equal(chunkDocIDs(ranked), want) {
		t.Fatalf("ranked = %v, want %v", chunkDocIDs(ranked), want)
	}
}

type rerankFakeChatModel struct {
	request ai.ModelRequest
	content string
}

func (m *rerankFakeChatModel) Generate(_ context.Context, request ai.ModelRequest) (ai.ModelResponse, error) {
	m.request = request
	return ai.ModelResponse{Message: ai.Message{Role: ai.RoleAssistant, Content: m.content}}, nil
}

func (m *rerankFakeChatModel) Stream(context.Context, ai.ModelRequest) (ai.TokenStream, error) {
	return nil, errors.New("not used")
}

func TestLLMRerankerBuildsPromptAndAppliesRanking(t *testing.T) {
	model := &rerankFakeChatModel{content: "<think>分析</think>\n3 1"}
	reranker := NewLLMReranker(model, nil)
	candidates := []ai.Chunk{rerankChunk("a"), rerankChunk("b"), rerankChunk("c")}

	ranked, err := reranker.Rerank(context.Background(), "Redis 超时", candidates)
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}
	want := []string{"c", "a", "b"}
	if !slices.Equal(chunkDocIDs(ranked), want) {
		t.Fatalf("ranked = %v, want %v", chunkDocIDs(ranked), want)
	}
	if len(model.request.Messages) != 2 {
		t.Fatalf("messages = %d, want system+user", len(model.request.Messages))
	}
	userContent := model.request.Messages[1].Content
	for _, fragment := range []string{"查询：Redis 超时", "[1] 标题：t-a", "[3] 标题：t-c", "content b"} {
		if !strings.Contains(userContent, fragment) {
			t.Fatalf("prompt missing %q in %q", fragment, userContent)
		}
	}
	if model.request.Messages[0].Role != ai.RoleSystem || !strings.Contains(model.request.Messages[0].Content, "不可信数据") {
		t.Fatalf("system prompt should declare untrusted data, got %q", model.request.Messages[0].Content)
	}
	if model.request.Options.Temperature != 0 {
		t.Fatalf("temperature = %v, want 0", model.request.Options.Temperature)
	}
}

func TestLLMRerankerFailsOnUnparseableOutput(t *testing.T) {
	reranker := NewLLMReranker(&rerankFakeChatModel{content: "这些片段看起来都相关"}, nil)
	if _, err := reranker.Rerank(context.Background(), "q", []ai.Chunk{rerankChunk("a")}); !errors.Is(err, ErrRerankUnparseable) {
		t.Fatalf("error = %v, want ErrRerankUnparseable", err)
	}
}

func chunkDocIDs(chunks []ai.Chunk) []string {
	ids := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		ids = append(ids, chunk.DocID)
	}
	return ids
}
