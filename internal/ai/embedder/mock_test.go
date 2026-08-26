package embedder

import (
	"context"
	"math"
	"testing"
)

func TestMockEmbedderIsDeterministicAndUnitNorm(t *testing.T) {
	embedder, err := NewMock(32)
	if err != nil {
		t.Fatal(err)
	}

	a, err := embedder.Embed(context.Background(), "Redis 超时排查")
	if err != nil {
		t.Fatal(err)
	}
	b, err := embedder.Embed(context.Background(), "Redis 超时排查")
	if err != nil {
		t.Fatal(err)
	}

	if len(a) != 32 {
		t.Fatalf("vector length = %d, want 32", len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("vector changes between calls at %d: %v vs %v", i, a[i], b[i])
		}
	}

	sum := 0.0
	for _, v := range a {
		sum += float64(v) * float64(v)
	}
	if math.Abs(sum-1) > 1e-5 {
		t.Fatalf("vector not unit norm, sum of squares = %v", sum)
	}
}

func TestMockEmbedderRejectsInvalidDim(t *testing.T) {
	if _, err := NewMock(0); err == nil {
		t.Fatal("NewMock(0) expected error")
	}
}
