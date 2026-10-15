# Pilot 项目导学

本导学按当前根模块源码整理，目标是让你能从真实代码讲清一条“运维问题 -> Agent/RAG -> 记忆持久化 -> 可观测定位 -> 故障修复”的链路。文档中的“待实施”代表当前仓库还没有对应运行证据。

## 1. 前置知识

| 知识点 | 为何需要 | 在本项目中的位置 | 高频度 |
| --- | --- | --- | --- |
| Go HTTP 与 context | 解释 Handler、超时、取消和优雅关闭 | `cmd/pilot/main.go`、`internal/handler/` | 极高 |
| Redis/MySQL 数据职责 | 解释短期窗口、长期事实和失败策略 | `internal/memory/`、`internal/memorydb/` | 极高 |
| SSE | 解释流式传输、flush、客户端断开 | `internal/handler/chat_stream.go`、`internal/chat/stream.go` | 高 |
| ES BM25/kNN/RRF | 解释混合检索和召回排序 | `internal/ai/retriever/` | 高 |
| Agent Loop 与工具白名单 | 解释模型决策、预算、回退和审计 | `internal/agent/`、`internal/tools/` | 极高 |
| Metrics/Trace/Logs | 解释 Prometheus、OpenTelemetry、Jaeger 的职责边界 | `docs/build/implementation/spec/11-observability-closed-loop.md` | 极高 |
| SLO 与故障演练 | 把“可观测”讲成工程闭环 | `docs/build/testing/README.md`、Stage 06 spec | 高 |

## 2. 重点亮点与学习顺序

| 亮点标题 | 为什么重要 | 通用技术关键词 | 先看哪些文件 | 建议学习顺序 |
| --- | --- | --- | --- | --- |
| 分层状态与双层记忆 | 能解释缓存和事实数据为什么分开 | cache-aside、事务、回源、TTL | `internal/memory/service.go`、`redis.go`、`mysql.go` | 1 |
| 流式取消与资源回收 | 能证明 SSE 不是简单循环打印 | context cancellation、flush、幂等 Close | `internal/chat/stream.go`、`internal/handler/chat_stream.go` | 2 |
| 混合检索与证据引用 | 能解释 BM25、向量和 RRF 的取舍 | RRF、去重、Top-K、引用 | `internal/ai/retriever/rrf.go`、`es_retriever.go`、`internal/ai/prompt/` | 3 |
| Agent 策略与工具治理 | 能回答无限循环、越权工具和失败回退 | budget、allowlist、fallback、audit | `internal/agent/loop.go`、`policy_runner.go`、`internal/tools/registry.go` | 4 |
| 可观测性闭环 | 能把 Prometheus 与 Jaeger 讲成排障流程 | SLI/SLO、Span、cardinality、OTLP | `docs/build/implementation/spec/11-observability-closed-loop.md` | 5 |

## 3. 必备知识点 checklist

- [ ] 能画出 `HTTP -> Handler -> chat.Service -> memory -> model/agent -> tool/retriever -> persistence`。
- [ ] 能说明 Redis 失败为什么不能伪装成空上下文，MySQL 写入为什么是事实边界。
- [ ] 能说明 SSE 正常 EOF、客户端取消、上游错误的不同收尾。
- [ ] 能说明 RRF 为什么按排名融合，而不是直接相加 BM25 和向量分数。
- [ ] 能说明 Agent 的最大 step、工具调用预算、重复失败和白名单如何共同停止循环。
- [ ] 能用一句话区分 Metrics、Trace、Logs，并说明 Jaeger 的接收端和 UI。
- [ ] 能执行一条故障演练：依赖停止 -> ready 失败 -> 指标增长 -> Trace 定位 -> 恢复验证。

## 4. 推荐阅读

| 主题 | 通用技术点 | 建议阅读位置 | 预计时间 | 读完能回答什么 |
| --- | --- | --- | ---: | --- |
| 启动与依赖 | 组合根、探针、优雅关闭 | `cmd/pilot/main.go`、`cmd/pilot/wiring.go`、`internal/deps/deps.go` | 30 分钟 | 服务如何启动，依赖失败如何处理？ |
| HTTP 契约 | 校验、错误映射、健康接口 | `internal/handler/*.go`、`docs/build/api/contract.md` | 30 分钟 | 为什么 Handler 要薄？ |
| 记忆持久化 | Redis 窗口、MySQL 事实、回源 | `internal/memory/service.go`、`internal/memory/redis.go`、`internal/memory/mysql.go` | 45 分钟 | 缓存和事实如何保持可解释？ |
| SSE | 分层流、取消传播、assistant 聚合 | `internal/chat/stream.go`、`internal/handler/chat_stream.go` | 45 分钟 | 客户端断开后如何停止下游？ |
| RAG | 文档切分、Embedding、RRF、ES | `internal/ai/retriever/`、`internal/ai/embedder/`、`internal/ai/prompt/` | 60 分钟 | 为什么需要混合检索？ |
| Agent | Runner、策略、工具、审计 | `internal/agent/`、`internal/tools/`、`internal/ai/eino/` | 60 分钟 | 如何限制模型并保证失败可控？ |
| 观测施工 | 指标、Span、日志、告警、演练 | `docs/build/implementation/spec/11-observability-closed-loop.md`、`docs/build/stages/stage-06-observability.md` | 60 分钟 | Prometheus 和 Jaeger 如何真正接上？ |
| 测试交付 | 单测、集成、冒烟、故障记录 | `docs/build/testing/README.md`、`stages/CHANGELOG.md` | 30 分钟 | 什么证据能证明闭环完成？ |

## 5. 自学提醒

若某文件或原理看不懂，请继续追问 AI；本技能负责给学习路径与题目，不提供逐行讲解。

## 6. 项目技术定位

后端 / AI 应用交叉项目：Go HTTP 服务承载聊天、SSE、Agent、RAG 和多层持久化，并以 Prometheus、OpenTelemetry、Jaeger 形成企业级排障方向的观测闭环。当前观测组件的部署声明已经存在，应用接线仍是下一阶段工作。

## 7. 核心原理解析

1. **问题 -> 机制 -> 落点：短期上下文和长期事实的访问特征不同。** Redis 保存最近窗口以降低读取延迟，MySQL 以事务保存完整历史；`memory.Service` 负责回源、回填和失败策略，避免 Handler 直接拼装两个存储。
2. **问题 -> 机制 -> 落点：流式响应中途断开会留下半截事实。** `chat.Service.Stream` 只在正常结束后聚合并保存 assistant，Handler 负责 SSE 编码和 flush；取消通过请求 context 向模型和工具传播。
3. **问题 -> 机制 -> 落点：BM25 与向量分数不可直接比较。** 两路结果按排名使用 RRF 融合、按 chunk 去重并截断 Top-K，Prompt 再加入来源标识，模型得到的是证据而不是无来源文本。
4. **问题 -> 机制 -> 落点：模型可能重复调用工具或调用越权工具。** Runner 使用最大 step、工具预算、参数大小、重复失败阈值和 fail-closed 白名单；ToolCall 审计保留状态、耗时和安全错误摘要。
5. **问题 -> 机制 -> 落点：只看平均延迟无法定位单次故障。** Prometheus 负责时间序列、趋势和告警，OpenTelemetry 负责跨层上下文，Jaeger 按 Trace ID 展开一次请求的 Span 树，结构化日志补充错误细节。当前这一条是施工目标，见 Stage 06 spec。

## 8. 关键设计决策

| 决策 | 备选 | 取舍 | 风险 | 验证 |
| --- | --- | --- | --- | --- |
| 先单体后微服务 | 直接拆多个服务 | 单体降低网络和部署成本，保留模块边界 | 边界可能继续膨胀 | Stage 07 闭环后再做 gRPC 演练 |
| Prometheus 拉取 `/metrics` | 应用主动推送所有指标 | 拉取模型简单、易调试、适合本地 | 高基数标签会拖垮 TSDB | 指标 review + cardinality 检查 |
| OTel OTLP -> Jaeger | 业务代码直接调用 Jaeger API | OTel 解耦供应商，未来可换 Tempo/Collector | exporter 配置或 shutdown flush 不完整 | Jaeger UI 查 root/child Span |
| Jaeger 作为 Trace 后端 | 用日志猜请求路径 | Trace 有父子关系和耗时 | Trace 采样可能丢长尾 | 失败演练按 trace_id 反查 |
| 观测失败默认降级 | 观测故障阻断业务 | 观测不应成为主链路单点 | 可能暂时缺少诊断证据 | ready degraded、限频告警和恢复测试 |

## 9. 量化与验证（含待测，建议）

- **待测可用性：** Compose 启动后 `/health/ready` 200，Prometheus `/-/ready` 200，Jaeger `/api/services` 能看到 `pilot`。
- **待测指标：** 一次 Chat 请求使 `pilot_http_requests_total` 增长 1，并产生一个延迟样本；路由标签只出现模板路径。
- **待测 Trace：** 一次请求至少有 HTTP root span 和一个业务子 span；Redis、MySQL、ES 调用失败时对应 Span 为 Error。
- **待测闭环：** 停 ES 后 ready 为 503、依赖错误计数上升、Jaeger 有错误 Span；恢复 ES 后 ready 回 200。
- **待测成本：** 记录 exporter 队列长度、丢弃数和 shutdown flush 耗时，再决定生产采样率和批量大小。
- **待测质量：** RAG 继续使用已有离线评估；观测指标只说明系统运行状态，不能替代召回率和答案质量评估。
