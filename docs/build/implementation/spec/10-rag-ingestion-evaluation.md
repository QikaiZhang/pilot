# RAG 知识库导入与评估规格

本文定义知识如何进入 Pilot，以及如何证明检索结果有用。它补充 [07-rag.md](07-rag.md) 的查询协议，不把“ES 能搜到文档”误认为 RAG 已经完成。

## 当前可运行链路

```text
POST /api/v1/knowledge/documents
  -> handler.KnowledgeHandler
  -> retriever.Ingestor
  -> SplitDocument
  -> Embedder
  -> ESStore.IndexMany(Bulk)

POST /api/v1/agent/chat
  -> Agent Runner
  -> knowledge_search
  -> ESRetriever(BM25 + kNN + RRF)
  -> tool observation
  -> 最终答案
```

本地可复现材料放在 `testdata/rag/corpus/`，包括 MySQL、Redis、Elasticsearch 三篇自有 runbook；`testdata/rag/eval/queries.jsonl` 保存检索问题和期望文档。生产环境的知识源应来自版本化文档仓库、对象存储或内部 Wiki，不应依赖服务容器内的本地文件。

当前代码已提供 `KnowledgeDocument`、`SplitDocument`、`Ingestor`、ES Bulk 写入和 `POST /api/v1/knowledge/documents`；真实 ES 集成测试使用唯一索引隔离，避免历史测试数据改变排名。这些能力证明了“导入和召回可运行”，不等于召回质量已经达标。

## 文档契约

导入输入至少包含 `doc_id/title/content/source/category/tags/version`。`title` 和 `content` 必填；`doc_id` 缺失时由 source+title 生成稳定 ID。版本号从 1 开始递增，同一 `doc_id+version` 重复导入必须幂等。

chunk 的 `chunk_id` 由 `doc_id、version、序号` 稳定生成，写入 ES 时作为 document ID。当前实现使用字符窗口（默认 1800 字符、重叠 200 字符），在半段落边界处优先切分；这是为了不引入特定 tokenizer 的教学版取舍。生产版本应使用与目标模型一致的 tokenizer，并按标题、段落、代码块和表格做结构化切分。

## 性能瓶颈与取舍

1. **Embedding 串行调用**：当前每个 chunk 依次调用 Embedder。文档较大时延迟近似线性增长；生产应使用批量 embedding、有限并发和速率限制，并保留失败 chunk 的重试队列。
2. **Bulk 刷新策略**：当前 `IndexMany` 使用 `refresh=true` 方便测试后立即检索。高吞吐导入会频繁刷新，生产应使用 `refresh=false`，由批次结束或定时任务刷新。
3. **chunk 重叠放大**：overlap 增大会提升边界召回，却会增加向量数量、索引空间和重复上下文；需要通过 Recall@K 与 token 成本共同调参。
4. **ES 客户端复用**：运行时应复用一个长生命周期 client；当前 Store 和 Retriever 各创建一个 client，功能正确但连接池无法完全共享，后续应改为组合根创建一次并注入。
5. **过滤后置**：`knowledge_search` 的 category 当前在召回后过滤，可能导致结果不足且浪费查询资源；生产应把 keyword filter 同时下推到 BM25 与 kNN 查询。

## 数据一致性与安全风险

- 重新导入新版本不会自动删除旧版本；生产需要版本状态、删除任务或 alias 切换，避免旧知识继续被召回。
- Bulk 响应当前只检查整体 `errors` 标记；生产需要解析每个 item 的失败 ID，支持部分重试和死信记录。
- 集成测试必须使用唯一索引或显式清理数据；共享测试索引会让旧文档改变 BM25/RRF 排名，产生假失败或假成功。
- 文档内容属于不可信证据，不能改变系统指令。Prompt 必须明确隔离外部内容，并限制工具结果的长度；答案应带来源标识，不能把“检索到”写成“已经执行”。
- 当前没有租户/权限字段。若知识库包含内部资料，必须把访问范围加入文档 metadata，并在两路检索中使用服务端过滤，不能让模型自行决定权限。
- 当前 mock embedder 只用于测试，不能代表真实语义质量；替换 embedding 模型或维度必须重建索引，不能混写。

## 评估数据与指标

评估集按 JSONL 保存。现有的 `expected_doc_ids` 仍然兼容，但完整样例可以增加相关性等级和答案约束：

```json
{
  "id": "redis-timeout-01",
  "query": "Redis 连接超时怎么排查？",
  "expected": [{"doc_id": "redis-timeout", "relevance": 3}],
  "answerable": true,
  "required_facts": ["检查实例存活", "检查网络连通性"],
  "required_citations": ["redis-timeout"],
  "language": "zh",
  "category": "cache"
}
```

### 1. 检索质量（程序精确计算）

| 指标 | 含义 | 关注点 |
| --- | --- | --- |
| HitRate@K | 前 K 是否至少命中一个相关片段 | 用户问题有没有被覆盖 |
| Recall@K | 相关片段有多少进入前 K | 知识是否被召回 |
| Precision@K | 前 K 中相关片段的比例 | 上下文是否被无关内容污染 |
| MRR | 第一个相关片段排名的倒数 | 最有用证据出现得是否足够靠前 |
| nDCG@K | 按 0-3 相关性等级评价排序 | 多个结果质量和顺序是否合理 |
| DuplicateRate@K | 前 K 中相同文档或重复 Chunk 的比例 | overlap 是否造成上下文浪费 |

`Recall@K` 适合判断“有没有召回”，`MRR/nDCG` 适合判断“排得好不好”，不能只看其中一个。每类问题至少准备 10 条，并按类别、语言、难度分别统计宏平均，避免总体平均数掩盖某一类完全失效。

### 2. 答案质量（人工或 LLM Judge）

检索命中不等于答案正确，生成评估至少记录：

- **Faithfulness**：答案中的事实是否都能被召回片段支持；
- **Answer Relevance**：答案是否真正回答了用户问题；
- **Citation Precision/Recall**：引用的来源是否支持结论，关键来源是否遗漏；
- **Correctness**：与人工参考答案或必备事实的符合程度；
- **Abstention**：知识不存在或证据不足时，是否明确说明不确定，而不是编造。

这组指标不是完全确定性的。LLM Judge 必须固定提示词、模型和温度，保存原始评分，并抽样人工复核；不能把 Judge 分数当作事实指标。

### 3. 运行质量（请求级与阶段级）

每次评估记录 `embedding_latency`、`bm25_latency`、`vector_latency`、`fusion_latency`、`total_latency`、输入/输出 token、工具失败率、空召回率和 BM25 fallback 率。批量导入另记录吞吐量、单 Chunk embedding 延迟、Bulk 部分失败数和索引刷新等待时间。

### 4. 安全与数据边界

安全指标不是平均分：任何一条越权召回、跨租户泄露或 Prompt 注入成功都应单独失败。评估集要包含无关问题、同义表达、中文口语、混合中英文、恶意指令、旧版本文档和无权限文档。

## 评估实现顺序

1. 先实现 JSONL 加载、`HitRate@K`、`Recall@K`、`Precision@K`、`MRR`、`nDCG@K` 和重复率的纯函数及表驱动测试。
2. 增加真实 ES 评估命令，输出机器可读 JSON 报告，并按类别输出宏平均和失败样例。
3. 补答案引用与 Faithfulness 评审数据；LLM Judge 放在检索基线稳定之后，并缓存评审结果。
4. 以基线做回归门禁，例如 Recall@5 下降超过约定阈值、p95 延迟显著上升或出现任何越权样例时阻止发布。具体阈值由实际基线决定，不在代码中写死。
5. 最后把稳定的运行指标接入 Prometheus；Prometheus 负责持续观测，离线评估负责判断检索和答案质量，二者不能互相替代。

## 实施顺序

1. 用 `testdata/rag/corpus` 调用上传接口，确认 chunk 数量、稳定 ID 和 ES 文档字段。
2. 运行真实 ES 集成测试，验证 Bulk 写入、BM25+kNN+RRF 和 category 过滤。
3. 将 `knowledge_search` 加入 Agent 白名单，验证工具审计、来源注入和无知识回退。
4. 补版本删除/alias、批量 embedding、服务端过滤和逐 item Bulk 错误处理。
5. 在上述事件稳定后接入 Prometheus，监控导入吞吐、检索质量代理指标、Agent 工具失败和 token 成本。
