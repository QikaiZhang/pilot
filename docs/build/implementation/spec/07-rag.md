# RAG 检索链路规格

本规格是 Stage 04 的施工依据：删除源码后，仅凭本文和 spec/02 数据模型即可重建「文档切分 -> Embedding -> ES 索引 -> 混合检索（RRF） -> 注入 Prompt」链路。

约定：本阶段用 Mock Embedder 打通链路，真实模型只替换 `Embedder` 实现，**不得改变 Controller、Memory、Retriever 和测试接口**。所有 RAG 接口均为项目自有接口，不暴露任何供应商类型。

## 自有接口契约

### Chunk：检索返回的一个文档片段

```go
type Chunk struct {
    DocID     string            // 源文档 ID
    ChunkID   string            // 片段 ID，唯一
    Title     string            // 标题
    Content   string            // 片段正文
    Source    string            // 来源（分类/标签过滤用）
    Category  string            // 分类，keyword 精确过滤
    Tags      []string          // 标签，keyword 精确过滤
    Version   int               // 版本，keyword 精确过滤
    Score     float64           // RRF 融合后的相关度（越高越靠前）
}
```

### Embedder：把文本转成向量

```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    Dim() int
}
```

- `Embed` 输入一段文本，返回一个长度等于 `Dim()` 的向量。索引侧用它编码 chunk 正文，查询侧用它编码用户 query。
- 索引侧与查询侧必须用同一个 `Embedder`（同模型、同维度），否则向量检索无效。
- Mock 实现：对文本做稳定 hash（同一文本恒得同一向量），归一化为单位向量，维度 `EMBEDDING_DIM`。

### Retriever：对单个 query 做混合召回

```go
type Retriever interface {
    Retrieve(ctx context.Context, query string, topK int) ([]Chunk, error)
}
```

- `Retrieve` 同时做关键词（BM25）和向量（kNN）召回，各取 `topK`，再经 RRF 融合并按融合分数降序返回。
- 召回结果为空是正常结果，返回 `[]Chunk{}`（非 nil）且 `error == nil`；只有在 ES 不可用或超时等使用方失败时才返回 error。
- 每次召回都要记录：命中的 `ChunkID`、命中的工具/路径、检索耗时。

## ES 索引约定

索引名取自 `ES_KNOWLEDGE_INDEX`（默认 `pilot_knowledge`）。mapping 见 [02-data-model.md](02-data-model.md)，要点：

- 全文检索字段用 `text`（`content`、`title`），会分词。
- 精确过滤字段用 `keyword`（`doc_id/chunk_id/category/source/version/tags`）。
- 向量字段 `vector` 用 `dense_vector`，`dims` 必须等于 `embedder.Dim()`；`similarity` 用 `cosine`。
- 模型或维度变更时，新建版本索引、批量重建、切换 alias，不得在同一个向量字段混写不同维度。

## RRF 融合规则（S 级手写）

BM25 分数和向量相似度量纲不同、不可直接比较，按排名融合：

```text
rrf_score(doc) = Σ   1 / (k + rank(doc))    （k 通常取 60）
                list∈{BM25, kNN}

最终按 rrf_score 降序，并以 ChunkID 去重。
```

- k 参数：`k=60` 是业界常用值。
- 去重以 `ChunkID`（或 `DocID+ChunkID`）为准，同一片段只保留一次。
- 过滤：召回后可再按 `category/source/tags` 精确过滤（keyword 匹配）。

## 失败策略

- ES 不可用 / 超时：返回可重试错误（wrap 上层 `retrieve...`），不得返回半截结果。
- 空召回：返回 `[]Chunk{}`，视为正常（可能只是知识库确实没相关内容）。
- Embedding 失败：阻断整个检索，原样向上抛错。

## 验证

- Mock Embedder：同一文本恒定输出，归一化后可复现，维度 = `EMBEDDING_DIM`。
- RRF：给定两个相同片段，BM25 第 1、kNN 第 1 的片段融合分高于另一片段。
- 检索链路：切分 -> embed -> 索引 -> 查询 -> RRF -> Top-K，端到端在本地 ES 上跑通。
