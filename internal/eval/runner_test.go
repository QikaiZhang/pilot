package eval

import (
	"context"
	"errors"
	"testing"

	"Pilot/internal/ai"
)

// fakeRetriever 为测试返回固定召回或指定错误。
type fakeRetriever struct {
	byQuery map[string][]ai.Chunk
	err     error
}

func (f fakeRetriever) Retrieve(_ context.Context, query string, topK int) ([]ai.Chunk, error) {
	if f.err != nil {
		return nil, f.err
	}
	chunks := f.byQuery[query]
	if len(chunks) > topK {
		chunks = chunks[:topK]
	}
	return chunks, nil
}

func TestNewRunner_RequiresRetriever(t *testing.T) {
	if _, err := NewRunner(nil, 0); err == nil {
		t.Fatal("expected error for nil retriever")
	}
}

func TestEvaluate_ComputesHitAndRetrieved(t *testing.T) {
	retriever := fakeRetriever{byQuery: map[string][]ai.Chunk{
		"redis": {{DocID: "redis-timeout", ChunkID: "r1"}, {DocID: "mysql-replication", ChunkID: "m1"}},
	}}
	runner, err := NewRunner(retriever, 0)
	if err != nil {
		t.Fatal(err)
	}
	queries := []Query{
		{ID: "q1", Query: "redis", Expected: []ExpectedDoc{{DocID: "redis-timeout", Relevance: 1}}},
		{ID: "q2", Query: "nope", Expected: []ExpectedDoc{{DocID: "missing"}}},
	}
	ev, err := runner.Evaluate(context.Background(), queries, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Cases[0].Hit {
		t.Fatalf("case[0].Hit = false, want true")
	}
	if len(ev.Results[0].Retrieved) != 2 || ev.Results[0].Retrieved[0] != "redis-timeout" {
		t.Fatalf("Retrieved = %v", ev.Results[0].Retrieved)
	}
	if ev.Cases[1].Hit {
		t.Fatalf("case[1].Hit = true, want false")
	}
}

func TestEvaluate_ErrorBecomesMiss(t *testing.T) {
	retriever := fakeRetriever{err: errors.New("es down")}
	runner, err := NewRunner(retriever, 0)
	if err != nil {
		t.Fatal(err)
	}
	queries := []Query{
		{ID: "q1", Query: "redis", Expected: []ExpectedDoc{{DocID: "redis-timeout"}}},
	}
	ev, err := runner.Evaluate(context.Background(), queries, 5)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Cases[0].Err == "" {
		t.Fatalf("case[0].Err = empty, want error")
	}
	if len(ev.Results[0].Retrieved) != 0 {
		t.Fatalf("Retrieved = %v, want empty on error", ev.Results[0].Retrieved)
	}
	if ev.Cases[0].Hit {
		t.Fatalf("Hit = true, want false on error")
	}
}

func TestRunner_EmptyQueries(t *testing.T) {
	runner, err := NewRunner(fakeRetriever{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := runner.Evaluate(context.Background(), nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Results != nil && len(ev.Results) != 0 {
		t.Fatalf("expected empty results, got %d", len(ev.Results))
	}
}

func TestEvaluate_RejectsNonPositiveK(t *testing.T) {
	runner, err := NewRunner(fakeRetriever{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Evaluate(context.Background(), nil, 0); !errors.Is(err, ErrInvalidK) {
		t.Fatalf("Evaluate error = %v, want ErrInvalidK", err)
	}
}
