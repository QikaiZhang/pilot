# Stage 07：观测证据查询层规格

状态：设计完成，代码未实施。

## 1. 背景和目标

当前 Pilot 已经能够把自身的 Metrics、Trace 和日志发送到 Prometheus、Jaeger 和 Elasticsearch，但 Agent 仍然只能通过 `health_check` 和 `knowledge_search` 获取证据。健康检查只回答依赖是否能连通，不能回答延迟、错误率、慢调用、资源趋势或具体 Trace 的问题。

本阶段要增加一个受控的 **Evidence Query Layer**，让 Agent 能在权限、并发、超时、结果大小和数据新鲜度约束内查询真实运行证据：

```text
用户问题 / 告警
  -> Agent 生成排查意图
  -> 查询策略层校验主体、服务、环境、时间窗和查询成本
  -> Evidence Query Layer 并行查询 Prometheus / Jaeger / Logs
  -> 统一证据模型（来源、时间、时效、限制、权限范围）
  -> Agent 结合知识库生成带引用的分析
```

本阶段解决的是“Agent 能否拿到真实证据”，不负责替换 Prometheus、Jaeger 或日志后端。

## 2. 当前实现与参考源码边界

### 根模块当前状态

根模块 `cmd/pilot/wiring.go` 的 Agent Registry 目前只装配：

- `health_check`：直接执行 Redis `PING`、MySQL `PingContext`、Elasticsearch `GET /`；
- `knowledge_search`：查询 Elasticsearch 知识索引。

因此当前 Agent 不能查询 PromQL、不能搜索慢 Trace、不能按时间窗查询运行日志。Prometheus 和 Jaeger 目前只服务于 Pilot 自身的监控和人工排障。

### `源代码/` 参考实现

`源代码/pilot_v 2/` 已提供可复用的结构草图：

| 参考位置 | 内容 | 可借鉴部分 | 必须重做部分 |
| --- | --- | --- | --- |
| `internal/ai/tools/sources.go` | `AlertsSource`、`MetricsSource`、`LogsSource`、`TraceSource` 接口 | 数据源端口隔离 | 增加权限上下文、预算和证据元数据 |
| `internal/ai/tools/observability_tools.go` | 查询告警、指标、日志、Trace 工具 | Agent 工具输入输出形状 | 禁止自由查询、增加结果上限和部分结果语义 |
| `internal/observability/prometheus_client.go` | Prometheus HTTP client | API 解析和时间范围处理 | 查询成本、服务范围、超时和响应字节限制 |
| `internal/observability/trace_client.go` | Jaeger Trace ID 查询 | Span 转项目模型 | 增加 Trace 搜索、采样/存储限制和官方 API 适配边界 |
| `internal/observability/collector.go` | DataCollector 聚合门面 | 统一查询入口 | 变成并发受控、可降级、带 freshness 的 evidence collector |

参考实现不能直接视为生产实现：它允许模型传入自由 PromQL，Trace 主要按 ID 查询，结果聚合没有统一的新鲜度和权限模型，且大响应存在内存和上下文放大风险。

## 3. 核心概念：证据不是普通工具返回值

所有查询结果必须统一包装为 `EvidenceSet`。模型只能根据 `EvidenceSet` 中的 `items` 形成事实判断。

```text
EvidenceSet
  request_id
  investigation_id
  subject        // 调用主体，不使用用户请求中的裸 user_id 作为权限依据
  service
  environment
  query_window   // start / end
  fetched_at
  freshness      // fresh / stale / unknown
  items[]
  limitations[]
  partial        // 是否有数据源失败、超时或被截断
  authorization_scope
```

每个 `EvidenceItem` 至少包含：

```text
source          // prometheus / jaeger / logs / alert
kind            // metric / trace / log / alert
observed_at     // 数据本身的观测时间
retrieved_at    // Pilot 获取时间
identity        // metric identity / trace id / log id，经过脱敏
summary         // 给模型的短摘要
raw_reference   // 允许人工复查的后端引用，不把原始大 payload 塞进 prompt
confidence      // confirmed / partial / unknown
```

### 证据状态

| 状态 | 含义 | Agent 可以怎么说 |
| --- | --- | --- |
| `confirmed` | 查询成功，时间窗和权限完整，来源返回有效数据 | “Prometheus 在窗口内显示……” |
| `partial` | 某数据源超时、结果截断或只拿到部分序列 | “目前只能确认部分指标……” |
| `stale` | 数据超过允许新鲜度 | “数据可能已过期，不能代表当前状态……” |
| `unknown` | 没有数据、采样不足或无法判断 | “没有足够证据确认……” |

空结果不能自动等价为“系统正常”。必须区分 `no_data`、`backend_unavailable`、`permission_denied` 和 `query_rejected`。

## 4. 查询工具设计

第一版只开放语义化工具，不开放任意后端 DSL：

| 工具 | 输入 | 真实数据源 | 输出重点 |
| --- | --- | --- | --- |
| `query_alerts` | service、environment、status、severity、window | Prometheus Alert API / Alertmanager | firing/resolved、开始时间、标签、限制 |
| `query_metric_evidence` | signal、service、environment、window、resolution | Prometheus Query API | 最新值、趋势摘要、序列数、采样时间 |
| `search_trace_evidence` | service、environment、window、min_duration、error_only、limit | Jaeger QueryService 或受控 adapter | 慢 Trace/错误 Trace 的摘要和 trace_id |
| `get_trace_evidence` | trace_id | Jaeger QueryService | Span 树、慢节点、错误 Span、采样限制 |
| `search_log_evidence` | service、environment、level、keyword、window、limit | Elasticsearch/日志查询服务 | 错误摘要、trace_id、时间、截断信息 |

### Prometheus 查询限制

- 服务端根据 `signal` 选择预定义 PromQL 模板；模型只能传模板参数。
- 禁止模型传入任意 URL、远端地址或未审核 PromQL。
- 每次查询必须绑定 service/environment 授权范围，模板负责注入受控 label matcher。
- 时间窗默认 15 分钟，单次最多 1 小时；resolution 使用固定档位，不允许任意 step。
- 限制最大序列数、最大采样点、最大响应字节和 Prometheus evaluation timeout。
- 查询返回每条序列的 label identity，禁止把不同实例、区域或版本无提示地平均成一个值。
- 最新值为零、无样本和后端查询失败必须分别表示。

Prometheus 的 range query 能返回时间序列，但“查询求值时间”不能直接当成“底层采集时间”。证据必须保留采样时间、抓取状态和查询时间；Prometheus 的 HTTP API 也有自身 timeout 和 result limit，应用层还要设置更小的预算。[Prometheus HTTP API](https://prometheus.io/docs/prometheus/latest/querying/api/)

### Jaeger 查询限制

- `get_trace_evidence` 只接受严格格式的 Trace ID，并检查主体是否有权限访问对应 service/environment。
- `search_trace_evidence` 必须按 service、时间窗、operation、最小耗时、error 状态和 limit 查询，不能扫描全库。
- 优先使用稳定的 QueryService gRPC adapter；若本地版本只使用 HTTP JSON，则封装在 adapter 内并锁定版本，不能让业务层依赖 UI 内部 API。
- 每次最多返回固定数量的 Trace，每条 Trace 最多固定 Span 数和属性字节；超限返回 `truncated` limitation。
- 采样导致“没有搜到”时只能标记 `unknown`，不能说“没有慢请求”。

Jaeger 官方将 QueryService gRPC 标为稳定的程序化查询接口，把 UI 使用的 HTTP JSON API 标为内部接口，因此生产实现应优先走稳定接口。[Jaeger APIs](https://www.jaegertracing.io/docs/1.76/architecture/apis/)

### 日志查询限制

- 日志查询必须限制 service/environment、时间窗、level、关键词长度、结果数和响应字节。
- 结果默认只返回摘要、时间、level、service、trace_id 和安全字段；原始堆栈、prompt、密钥和大字段必须脱敏或截断。
- 日志后端不可用时返回 `backend_unavailable`，不能伪装成“没有错误日志”。

## 5. 性能设计

### 5.1 预算模型

每次 Agent investigation 创建 `QueryBudget`，预算不能由模型修改：

| 预算 | 默认值（建议） | 作用 |
| --- | ---: | --- |
| 总查询 deadline | 8s | 防止观测查询拖垮 Agent 总超时 |
| 单数据源 timeout | 2s | Prometheus、Jaeger、Logs 各自隔离 |
| 最大并行查询 | 3 | 一次最多并行查询三个数据源 |
| 最大工具调用 | 6 | 防止模型反复查询同一窗口 |
| 最大返回证据项 | 30 | 控制模型上下文和序列化成本 |
| 最大 Trace 数 | 5 | 防止大 Trace 扩散 |
| 最大日志数 | 20 | 防止日志结果淹没模型 |
| 最大响应字节 | 256 KiB | 保护 Pilot 内存和下游模型输入 |
| 最大 Prometheus 时间序列 | 20 | 防止高基数查询 |

预算消耗必须有指标：

```text
pilot_evidence_queries_total{source,kind,result}
pilot_evidence_query_duration_seconds{source,kind}
pilot_evidence_query_inflight{source}
pilot_evidence_query_rejected_total{reason}
pilot_evidence_items_total{source,kind}
pilot_evidence_bytes_total{source}
pilot_evidence_stale_total{source}
```

### 5.2 并发与故障隔离

```text
同一 investigation
  -> Prometheus query ┐
  -> Jaeger search    ├─ 独立 timeout、独立结果槽位
  -> Logs query       ┘
  -> 等待 deadline
  -> 合并 EvidenceSet
```

- 查询之间无依赖时并行执行；Trace 详情查询依赖先得到的 trace_id，必须单独进入第二阶段。
- 每个数据源使用 semaphore 限制全局并发；不能只限制单个 HTTP 请求。
- 一个数据源超时不能取消已成功的兄弟查询，结果标记 `partial` 并写入 limitation。
- Agent 总 deadline 到期时停止新查询，已经完成的证据仍可用于有限回答。
- 不对任意查询无条件重试；只有明确可重试的网络错误允许一次短退避重试，并消耗查询预算。
- 大结果使用流式解码或 `io.LimitReader`；达到字节上限后立即停止并标记截断。

### 5.3 缓存与请求合并

缓存只优化重复读取，不能改变证据语义：

- 告警缓存建议 5-15 秒；指标结果缓存建议按 resolution 设置 5-30 秒；Trace 详情缓存建议 30-120 秒；日志缓存建议 5-15 秒。
- 缓存键必须包含主体权限范围、service、environment、query kind、模板参数、时间窗和数据源版本。
- 不同主体不能共享未隔离的证据缓存。
- 对完全相同的并发查询使用 singleflight，请求取消不能取消仍被其他请求使用的共享查询。
- 缓存响应必须保留原始 `fetched_at` 和 freshness；命中缓存不等于重新获取。
- 故障恢复确认默认绕过缓存或使用极短 TTL，避免拿旧结果判断“已经恢复”。

## 6. 可用性和降级设计

### 6.1 数据源状态

| 数据源状态 | 行为 | Agent 输出约束 |
| --- | --- | --- |
| healthy | 正常查询 | 可以引用证据 |
| slow | 超过软阈值但未超时 | 标记 latency limitation |
| unavailable | 连接失败、5xx、超时 | 保留其他来源，明确缺口 |
| unauthorized | 权限不足 | 不泄露后端存在性细节 |
| stale | 只能返回过期缓存 | 禁止当作当前事实 |
| empty | 查询成功但无匹配数据 | 明确“无匹配数据”，不说“正常” |

### 6.2 组合结果策略

结果组合采用“部分成功优先”：

```text
Prometheus 成功 + Jaeger 超时 + Logs 成功
  -> 返回指标和日志
  -> partial=true
  -> limitations=[jaeger_timeout]
  -> Agent 只能给有限结论
```

以下情况禁止生成确定性根因：

- 所有实时数据源都失败；
- 只有过期缓存，没有当前数据；
- 只有知识库文档，没有运行证据；
- 查询结果被截断且截断部分可能影响结论；
- Trace 被采样或权限裁剪，无法确认完整调用树。

### 6.3 观测系统故障不能拖垮主业务

- Evidence Query Layer 是 Agent 的可选能力，查询失败应返回稳定 limitation 或有限回答，不阻塞健康接口、普通 Chat 和已有知识库能力。
- 查询客户端必须使用独立 HTTP transport、连接池、timeout 和并发 semaphore，不能与主业务外部调用共享无界资源。
- Prometheus、Jaeger、日志后端不可用时，Pilot 自身的 metrics 和 root Trace 仍要记录 evidence query failure。
- 进入持续故障状态后可以短暂熔断某个数据源，使用半开探测恢复，避免每个用户请求都打到故障后端。

## 7. 实时性与新鲜度契约

每条 EvidenceSet 必须包含：

```text
query_window.start / end
fetched_at
source_sample_time 或 trace_start_time
freshness_ttl
freshness_status
scrape_or_export_lag（如果数据源可提供）
```

建议新鲜度规则：

| 类型 | 新鲜度目标 | 过期后的行为 |
| --- | ---: | --- |
| firing alert | 15s | 标记 stale，重新查询确认 |
| latency/error metric | 30s | 可用于趋势，不可直接声称当前瞬时状态 |
| Trace search | 30s | 新 Trace 可能仍在导出，标记 possible_lag |
| Trace detail | 2m | 允许短缓存，故障恢复确认绕过缓存 |
| error logs | 30s | 标记 ingestion_lag unknown |
| knowledge document | 版本级 | 不属于实时证据，作为背景知识 |

“实时”在这里定义为：**在明确的新鲜度预算内重新查询并携带时间证据**，不是每个 token 都推送监控数据。

## 8. 权限层与性能层的交叉边界

权限检查必须在查询发出前完成，避免用后端错误来代替授权：

```text
认证主体
  -> 读取服务端授权上下文
  -> 检查 service/environment/signal/action
  -> 生成受控查询模板
  -> 加入数据源客户端凭证
  -> 执行带预算的查询
```

缓存、singleflight、并发池都必须把权限范围作为隔离维度。一个高权限请求不能让低权限请求命中相同结果。

Prometheus 和 Jaeger 管理端点不能直接暴露给模型或公网。Pilot 只允许访问固定配置的后端地址，禁止模型提供 URL；Prometheus 官方也提醒其 HTTP API 和 metrics 端点包含系统信息并可能被查询打垮。[Prometheus Security Model](https://prometheus.io/docs/operating/security/)

## 9. 实施阶段

### P0：受控 Prometheus 证据

- 增加 `MetricsSource` 和 `EvidenceCollector` 项目接口。
- 只实现 3-5 个固定 signal：request rate、5xx ratio、p95 latency、dependency error、inflight。
- 加主体授权上下文、固定 PromQL 模板、时间窗/序列/字节/timeout 限制。
- 返回 `EvidenceSet`，覆盖 fresh、empty、timeout、backend unavailable 和 partial。

### P1：Jaeger Trace 关联

- 增加按 service/time window/error/min duration 搜索 Trace 的 adapter。
- 增加 trace_id 详情查询和 Span 数/属性字节上限。
- 处理采样、导出延迟、查询结果截断和权限裁剪。
- 完成“Prometheus 发现异常 -> Jaeger 定位慢 Span”的集成测试。

### P2：日志证据与查询并发治理

- 接入日志查询 source，统一脱敏、摘要、limit 和 bytes budget。
- 增加全局 semaphore、singleflight、短 TTL 缓存和按数据源熔断。
- 增加 evidence query metrics、故障演练和恢复确认绕过缓存的测试。

### P3：Agent 排查计划与闭环

- Agent 按固定排查计划选择 evidence tools，不允许自由拼接后端 DSL。
- 结论强制引用 evidence ID，区分 confirmed、partial、stale、unknown。
- 实现一个真实场景：服务延迟升高 -> 指标异常 -> Trace 慢节点 -> 日志错误 -> 恢复后二次确认。

## 10. 风险清单

| 风险 | 类型 | 优先级 | 处理 |
| --- | --- | --- | --- |
| Agent 任意 PromQL 导致高成本查询 | Introduced | P0 | 固定 signal 模板和查询预算 |
| 多用户并发查询打垮 Prometheus/Jaeger | Introduced | P0 | 全局 semaphore、deadline、熔断 |
| 旧快照复用过期运行证据 | Amplified | P0 | 证据单独 freshness TTL，恢复确认绕过缓存 |
| Jaeger 采样导致误判“无慢请求” | Existing | P1 | 返回 sampling limitation，禁止确定性结论 |
| 大 Trace/日志响应撑爆 Pilot 内存 | Introduced | P1 | 流式解码、字节/条数/Span 限制 |
| 后端短暂故障导致 Agent 全部失败 | Introduced | P1 | 部分结果、数据源隔离、有限降级 |
| 高权限证据进入低权限缓存 | Introduced | P0 | 缓存键绑定 authorization scope |
| 指标平均值掩盖实例/版本差异 | Existing | P1 | 保留 label identity，服务端显式聚合 |

## 11. Definition of Done

- [ ] Agent 可以通过受控工具查询至少三类 Prometheus 运行证据。
- [ ] 任意 PromQL、URL、Trace ID、服务和环境都经过服务端约束与授权。
- [ ] 单请求有总 deadline、单源 timeout、并发上限、结果条数和字节上限。
- [ ] Prometheus、Jaeger、Logs 任一数据源失败时，其余证据仍可返回并带 limitation。
- [ ] EvidenceSet 携带 fetched_at、sample/trace 时间、freshness 和权限范围。
- [ ] 缓存和 singleflight 按授权范围隔离；恢复确认不会误用旧证据。
- [ ] 至少一个故障演练完成：延迟升高 -> 指标 -> Trace -> 日志 -> 恢复确认。
- [ ] 面试口播能解释性能、可用性、实时性和权限之间的取舍。

## 12. 面试口播

“我不会让模型直接访问 Prometheus 或 Jaeger，也不会把任意 PromQL 当成工具参数。我的做法是增加一个证据查询层：先用认证主体确定能查哪些服务和环境，再把模型的排查意图映射成固定查询模板。每次调查都有总 deadline、数据源独立超时、并发 semaphore、结果条数和响应字节预算。Prometheus、Jaeger、日志三个数据源可以并行查询，单个数据源超时不取消其他结果，而是返回 partial limitation。每条证据都带查询时间窗、获取时间、观测时间和 freshness，缓存命中也必须保留原始 fetched_at。这样 Agent 的结论能区分已确认事实、部分证据、过期数据和未知状态；系统也不会因为模型的一次高成本查询或某个监控后端故障而拖垮主链路。”
