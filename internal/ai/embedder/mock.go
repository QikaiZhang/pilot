// Package embedder 提供文本向量化能力。
// 本阶段只包含确定性 Mock 实现，用于打通索引、检索和 RRF 链路，
// 不依赖任何真实 embedding 模型；真实模型只替换 Embedder 实现即可。
package embedder

import (
	"context"
	"errors"
	"hash/fnv"
	"math"
	"strings"
)

// errInvalidDim 表示向量维度必须为正数。
var errInvalidDim = errors.New("embedder dimension must be positive")

// MockEmbedder 是确定性文本向量化实现：同一文本恒得同一向量，
// 并归一化为单位向量。它不代表真实语义质量，只用于验证
// 索引、召回、RRF 和错误路径。
type MockEmbedder struct {
	dim int
}

// NewMock 构造指定维度的 Mock Embedder。
func NewMock(dim int) (*MockEmbedder, error) {
	if dim <= 0 {
		return nil, errInvalidDim
	}
	return &MockEmbedder{dim: dim}, nil
}

// Dim 返回向量维度。
func (m *MockEmbedder) Dim() int {
	if m == nil {
		return 0
	}
	return m.dim
}

// Embed 把文本映射为定长向量。char 位置决定该维的权重方向，
// 用 FNV-1a 对每个字符编码，保证相同文本可复现。
func (m *MockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if m == nil || m.dim <= 0 {
		return nil, errInvalidDim
	}
	vec := make([]float32, m.dim)
	hasher := fnv.New32a()
	// 逐字符散列：稳定、快速、确定；空文本返回全零向量。
	for _, r := range strings.TrimSpace(text) {
		_, _ = hasher.Write([]byte(string(r)))
		index := hasher.Sum32() % uint32(m.dim)
		vec[index] += 1
	}
	// 归一化为单位向量，避免不同长度文本的向量模长影响余弦相似度。
	norm := 0.0
	for _, v := range vec {
		norm += float64(v * v)
	}
	if norm == 0 {
		return vec, nil
	}
	scale := 1 / math.Sqrt(norm)
	for i, v := range vec {
		vec[i] = float32(float64(v) * scale)
	}
	return vec, nil
}
