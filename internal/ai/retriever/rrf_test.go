package retriever

import (
	"math"
	"testing"

	"Pilot/internal/ai"
)

func chunk(id string) ai.Chunk {
	return ai.Chunk{DocID: "doc-" + id, ChunkID: id}
}

func TestFuseRRFPrefersFragmentPresentInBothLists(t *testing.T) {
	// BM25 排名: a(1), b(2), c(3)
	// 向量排名: b(1), d(2)
	bm25 := []ai.Chunk{chunk("a"), chunk("b"), chunk("c")}
	vector := []ai.Chunk{chunk("b"), chunk("d")}

	got := fuseRRF(bm25, vector)

	if len(got) != 4 {
		t.Fatalf("fuseRRF() length = %d, want 4", len(got))
	}
	// b 同时出现在两路且都是第 1/第 2,融合分最高。
	if got[0].ChunkID != "b" {
		t.Fatalf("top = %q, want b (present in both lists)", got[0].ChunkID)
	}
	// 跨来源加分: b 的分数应大于仅出现一次的 a。
	if got[0].Score <= got[1].Score {
		t.Fatalf("b score = %v, a score = %v; cross-source should win", got[0].Score, got[1].Score)
	}
}

func TestFuseRRFIsDescendingByScore(t *testing.T) {
	bm25 := []ai.Chunk{chunk("x"), chunk("y"), chunk("z")}

	got := fuseRRF(bm25, nil)
	for i := 1; i < len(got); i++ {
		if got[i-1].Score < got[i].Score {
			t.Fatalf("scores not descending at %d: %v then %v", i, got[i-1].Score, got[i].Score)
		}
	}
}

func TestFuseRRFDeduplicatesSameSource(t *testing.T) {
	bm25 := []ai.Chunk{chunk("a"), chunk("a"), chunk("b")}

	got := fuseRRF(bm25, nil)
	if len(got) != 2 {
		t.Fatalf("fuseRRF() length = %d, want 2 (dedup same source)", len(got))
	}
}

func TestFuseRRFContributionUsesRank(t *testing.T) {
	// 排名 1 贡献 1/(60+1),排名 2 贡献 1/(60+2)。
	list := []ai.Chunk{chunk("first"), chunk("second")}

	got := fuseRRF(list, nil)
	wantFirst := 1.0 / float64(rrfK+1)
	if math.Abs(got[0].Score-wantFirst) > 1e-9 {
		t.Fatalf("first score = %v, want %v", got[0].Score, wantFirst)
	}
}
