package eval

import (
	"math"
	"testing"

	"Pilot/internal/ai"
)

func TestHitRateAtK(t *testing.T) {
	results := []QueryResult{
		{Expected: map[string]float64{"a": 1}, Retrieved: []string{"a", "b"}},
		{Expected: map[string]float64{"c": 1}, Retrieved: []string{"a", "b"}},
		{Expected: map[string]float64{"b": 1}, Retrieved: []string{"b"}},
	}
	if got, want := HitRateAtK(results), 2.0/3.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("HitRateAtK = %v, want %v", got, want)
	}
}

func TestRecallAtK(t *testing.T) {
	results := []QueryResult{
		{Expected: map[string]float64{"a": 1, "b": 1}, Retrieved: []string{"a"}},
		{Expected: map[string]float64{"c": 1}, Retrieved: []string{"c"}},
		{Expected: map[string]float64{"d": 1}, Retrieved: nil},
	}
	if got, want := RecallAtK(results), 0.5; math.Abs(got-want) > 1e-9 {
		t.Fatalf("RecallAtK = %v, want %v", got, want)
	}
}

func TestPrecisionAtK(t *testing.T) {
	results := []QueryResult{
		{Expected: map[string]float64{"a": 1}, Retrieved: []string{"a", "b"}},
		{Expected: map[string]float64{"c": 1}, Retrieved: []string{"b", "d"}},
		{Expected: map[string]float64{"c": 1}, Retrieved: []string{"c", "x", "y"}},
	}
	// 命中总数 2，分母 = 3 条 query × K(3) = 9。
	if got, want := PrecisionAtK(results, 3), 2.0/9.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("PrecisionAtK = %v, want %v", got, want)
	}
}

func TestMRR(t *testing.T) {
	results := []QueryResult{
		{Expected: map[string]float64{"a": 1}, Retrieved: []string{"a", "b"}},
		{Expected: map[string]float64{"c": 1}, Retrieved: []string{"a", "b"}},
		{Expected: map[string]float64{"c": 1}, Retrieved: []string{"x", "c"}},
	}
	// 1 + 0 + 1/2 再除以 3。
	if got, want := MRR(results), (1.0+0.0+0.5)/3.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("MRR = %v, want %v", got, want)
	}
}

func TestNDCGAtK_HandlesGradeAndOrder(t *testing.T) {
	t.Run("ideal order scores 1", func(t *testing.T) {
		results := []QueryResult{
			{Expected: map[string]float64{"a": 3, "b": 1}, Retrieved: []string{"a", "b"}},
		}
		if got, want := NDCGAtK(results, 2), 1.0; math.Abs(got-want) > 1e-9 {
			t.Fatalf("NDCGAtK = %v, want %v", got, want)
		}
	})
	t.Run("ideal ranking selects highest grades before truncating", func(t *testing.T) {
		results := []QueryResult{
			{Expected: map[string]float64{"low": 1, "medium": 2, "high": 3}, Retrieved: []string{"high", "medium"}},
		}
		if got := NDCGAtK(results, 2); math.Abs(got-1) > 1e-9 {
			t.Fatalf("NDCGAtK = %v, want 1", got)
		}
	})
	t.Run("wrong order hurts score", func(t *testing.T) {
		results := []QueryResult{
			{Expected: map[string]float64{"a": 3, "b": 1}, Retrieved: []string{"b", "a"}},
		}
		got := NDCGAtK(results, 2)
		if got >= 1.0 || got <= 0 {
			t.Fatalf("NDCGAtK = %v, want in (0,1)", got)
		}
	})
	t.Run("no relevant returns 0", func(t *testing.T) {
		results := []QueryResult{
			{Expected: map[string]float64{"a": 3}, Retrieved: []string{"z"}},
		}
		if got := NDCGAtK(results, 2); got != 0 {
			t.Fatalf("NDCGAtK = %v, want 0", got)
		}
	})
	t.Run("empty expected returns 0", func(t *testing.T) {
		results := []QueryResult{
			{Expected: map[string]float64{}, Retrieved: []string{"a"}},
		}
		if got := NDCGAtK(results, 2); got != 0 {
			t.Fatalf("NDCGAtK = %v, want 0", got)
		}
	})
}

func TestDuplicateRateAtK(t *testing.T) {
	results := []QueryResult{
		{DuplicateRatio: 0.0},
		{DuplicateRatio: 0.5},
		{DuplicateRatio: -1}, // 未统计，应被跳过
	}
	if got, want := DuplicateRateAtK(results), 0.25; math.Abs(got-want) > 1e-9 {
		t.Fatalf("DuplicateRateAtK = %v, want %v", got, want)
	}
}

func TestUnpackDocIDs_DeduplicatesAndCapsAtK(t *testing.T) {
	chunks := []ai.Chunk{
		{DocID: "a", ChunkID: "1"},
		{DocID: "a", ChunkID: "2"},
		{DocID: "b", ChunkID: "3"},
		{DocID: "c", ChunkID: "4"},
	}
	got := UnpackDocIDs(chunks, 3)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDuplicateRatio(t *testing.T) {
	cases := []struct {
		name   string
		chunks []ai.Chunk
		k      int
		want   float64
	}{
		{"duplicate chunk id", []ai.Chunk{{ChunkID: "1"}, {ChunkID: "1"}, {ChunkID: "2"}}, 3, 1.0 / 3.0},
		{"duplicate doc id", []ai.Chunk{{DocID: "a", ChunkID: "1"}, {DocID: "a", ChunkID: "2"}, {ChunkID: "3"}}, 3, 1.0 / 3.0},
		{"no duplicates", []ai.Chunk{{ChunkID: "1"}, {ChunkID: "2"}}, 3, 0},
		{"empty", nil, 3, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DuplicateRatio(tc.chunks, tc.k); math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("DuplicateRatio = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSummary_EmptyResults(t *testing.T) {
	s := Summary(nil, 5)
	if s.N != 0 || s.HitRateAtK != 0 || s.RecallAtK != 0 {
		t.Fatalf("Summary of empty = %+v, want zeros", s)
	}
}
