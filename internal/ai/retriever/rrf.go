// Package retriever 提供知识库混合召回。
// 它把关键词（BM25）与向量（kNN）两路召回按排名融合（RRF），
// 屏蔽各来源分数量纲不一致的问题。
package retriever

import (
	"sort"

	"Pilot/internal/ai"
)

// rrfK 是 RRF 融合常数，用于压缩排名差：排名第 1 的贡献 1/(k+1)。
// k 越大排名差的影响越小；业界常用 60。
const rrfK = 60

// fuseRRF 融合多路（按各自相关度降序）的召回结果：
// 每个片段在各来源中按排名贡献 1/(rrfK+rank)，以 ChunkID 累加并去重，
// 最后按融合分降序返回。
func fuseRRF(lists ...[]ai.Chunk) []ai.Chunk {
	scores := make(map[string]*ai.Chunk)

	for _, list := range lists {
		// 同一来源内按排名去重：同 ChunkID 只保留最靠前的一次。
		rankByChunk := make(map[string]int)
		ordered := make([]string, 0, len(list))
		for rank, chunk := range list {
			key := chunk.ChunkID
			if key == "" {
				key = chunk.DocID
			}
			if _, ok := rankByChunk[key]; ok {
				continue
			}
			rankByChunk[key] = rank + 1
			ordered = append(ordered, key)
		}
		// 按本来源的排名累加贡献。
		for _, key := range ordered {
			rank := rankByChunk[key]
			contribution := 1.0 / float64(rrfK+rank)
			if existing, ok := scores[key]; ok {
				existing.Score += contribution
			} else {
				// 以出现该片段的第一路数据为准，Score 从 0 累加。
				copyChunk := headline(list, key)
				copyChunk.Score = contribution
				scores[key] = &copyChunk
			}
		}
	}

	result := make([]ai.Chunk, 0, len(scores))
	for _, chunk := range scores {
		result = append(result, *chunk)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})
	return result
}

// headline 返回 list 里 ChunkID（或 DocID）等于 key 的第一个片段。
func headline(list []ai.Chunk, key string) ai.Chunk {
	for _, c := range list {
		id := c.ChunkID
		if id == "" {
			id = c.DocID
		}
		if id == key {
			return c
		}
	}
	return ai.Chunk{}
}
