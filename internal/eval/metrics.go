package eval

import (
	"math"

	"Pilot/internal/ai"
)

// QueryResult 是单条 query 的评估输入：期望命中的文档，以及实际召回的有序 doc 列表。
// Retrieved 已经按 doc_id 去重并保证是倒序（rank 0 最相关）。
type QueryResult struct {
	ID             string
	Expected       map[string]float64 // doc_id -> relevance
	Retrieved      []string           // 按相关度降序去重后的 doc_id
	DuplicateRatio float64            // 前 K 的 chunk 重复率，由 runner 填充（0 表示未统计）
}

const maxRelevance = 3

// HitRateAtK 返回「前 K 至少命中一个相关文档」的 query 占比。
// 它衡量用户问题有没有被覆盖，是所有指标的前提。
func HitRateAtK(results []QueryResult) float64 {
	if len(results) == 0 {
		return 0
	}
	hit := 0
	for _, r := range results {
		for _, docID := range r.Retrieved {
			if _, ok := r.Expected[docID]; ok {
				hit++
				break
			}
		}
	}
	return float64(hit) / float64(len(results))
}

// RecallAtK 返回相关文档有多少比例进入前 K。
// 分母是所有期望文档总数，衡量「知识有没有被召回」。
func RecallAtK(results []QueryResult) float64 {
	total := 0
	recalled := 0
	for _, r := range results {
		for docID := range r.Expected {
			total++
			if contains(r.Retrieved, docID) {
				recalled++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(recalled) / float64(total)
}

// PrecisionAtK 返回前 K 个结果中相关文档的比例，分母是 K。
// Recalled 不足 K（知识库候选少）时仍按 K 计算，保证该指标跨 query 可比。
func PrecisionAtK(results []QueryResult, k int) float64 {
	if len(results) == 0 || k <= 0 {
		return 0
	}
	correct := 0
	for _, r := range results {
		retrieved := r.Retrieved
		if len(retrieved) > k {
			retrieved = retrieved[:k]
		}
		for _, docID := range retrieved {
			if _, ok := r.Expected[docID]; ok {
				correct++
			}
		}
	}
	return float64(correct) / float64(len(results)*k)
}

// MRR 返回首个相关文档排名的倒数平均，判断「最有用证据出现得够不够靠前」。
func MRR(results []QueryResult) float64 {
	if len(results) == 0 {
		return 0
	}
	sum := 0.0
	for _, r := range results {
		for rank, docID := range r.Retrieved {
			if _, ok := r.Expected[docID]; ok {
				sum += 1 / float64(rank+1)
				break
			}
		}
	}
	return sum / float64(len(results))
}

// NDCGAtK 按相关性等级（默认 binary）评价排序质量。
// 期望等级缺失时按 relevance=1 计算，因此对 binary 标注仍有效。
func NDCGAtK(results []QueryResult, k int) float64 {
	if len(results) == 0 || k <= 0 {
		return 0
	}
	sum := 0.0
	for _, r := range results {
		sum += ndcgForQuery(r, k)
	}
	return sum / float64(len(results))
}

func ndcgForQuery(r QueryResult, k int) float64 {
	ideal := idealDCG(r.Expected, k)
	if ideal == 0 {
		return 0
	}
	dcg := 0.0
	for rank := 0; rank < len(r.Retrieved) && rank < k; rank++ {
		rel, ok := r.Expected[r.Retrieved[rank]]
		if !ok {
			rel = 0
		}
		dcg += rel / math.Log2(float64(rank+2))
	}
	return dcg / ideal
}

func idealDCG(expected map[string]float64, k int) float64 {
	rels := make([]float64, 0, len(expected))
	for _, rel := range expected {
		rel = normalizeRelevance(rel)
		rels = append(rels, rel)
	}
	// 先排序再截断，才能取出前 K 个最高相关性等级作为理想排序。
	sortDescending(rels)
	if len(rels) > k {
		rels = rels[:k]
	}
	dcg := 0.0
	for rank, rel := range rels {
		dcg += rel / math.Log2(float64(rank+2))
	}
	return dcg
}

// normalizeRelevance 把任意非负等级压缩到 0..maxRelevance。
func normalizeRelevance(rel float64) float64 {
	if rel <= 0 {
		return 0
	}
	if rel > maxRelevance {
		return maxRelevance
	}
	return rel
}

// DuplicateRateAtK 返回每个 query 前 K 的 chunk 重复率均值。
// 重复判定在 runner 里基于原始 chunk 列表（doc_id 或 chunk_id 出现多次）。
func DuplicateRateAtK(results []QueryResult) float64 {
	if len(results) == 0 {
		return 0
	}
	sum := 0.0
	counted := 0
	for _, r := range results {
		if r.DuplicateRatio < 0 {
			continue
		}
		sum += r.DuplicateRatio
		counted++
	}
	if counted == 0 {
		return 0
	}
	return sum / float64(counted)
}

// UnpackDocIDs 把召回片段按 doc_id 去重、保序并截断到 K。
// 同一个文档出现在多个 chunk 时只保留最高位次，避免放大同一文档的权重。
func UnpackDocIDs(chunks []ai.Chunk, k int) []string {
	if len(chunks) == 0 || k <= 0 {
		return nil
	}
	seen := make(map[string]bool, len(chunks))
	docs := make([]string, 0, k)
	for _, c := range chunks {
		docID := c.DocID
		if docID == "" {
			docID = c.ChunkID
		}
		if seen[docID] {
			continue
		}
		seen[docID] = true
		docs = append(docs, docID)
		if len(docs) == k {
			break
		}
	}
	return docs
}

// DuplicateRatio 返回前 K 个片段中重复 chunk 或重复 doc 的数量占比。
// 用于衡量 overlap 或同一文档多 chunk 造成的上下文浪费。负值表示「未统计，跳过」。
func DuplicateRatio(chunks []ai.Chunk, k int) float64 {
	if len(chunks) == 0 || k <= 0 {
		return 0
	}
	window := chunks
	if len(window) > k {
		window = window[:k]
	}
	seenChunk := make(map[string]bool, len(window))
	seenDoc := make(map[string]bool, len(window))
	duplicates := 0
	for _, c := range window {
		isDup := false
		if c.ChunkID != "" {
			if seenChunk[c.ChunkID] {
				isDup = true
			}
			seenChunk[c.ChunkID] = true
		}
		if c.DocID != "" {
			if seenDoc[c.DocID] {
				isDup = true
			}
			seenDoc[c.DocID] = true
		}
		if isDup {
			duplicates++
		}
	}
	return float64(duplicates) / float64(len(window))
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func sortDescending(rels []float64) {
	for i := 1; i < len(rels); i++ {
		for j := i; j > 0 && rels[j] > rels[j-1]; j-- {
			rels[j], rels[j-1] = rels[j-1], rels[j]
		}
	}
}
