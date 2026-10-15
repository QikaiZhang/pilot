# Stage 06：Prometheus + OpenTelemetry + Jaeger 可观测性闭环规格

状态：设计完成，应用接线待实施。

这份规格解决当前最明显的断点：`manifest/docker-compose.yml` 已声明 Prometheus 与 Jaeger，但根模块的 `cmd/pilot` 尚未注册 `/metrics`、HTTP 观测中间件、OpenTelemetry SDK 或 OTLP exporter。因此容器可以启动，页面却没有 Pilot 的指标和 Trace。

## 1. 先把三个概念讲清楚

| 能力 | 回答的问题 | 数据形态 | 本项目的用途 |
| --- | --- | --- | --- |
| Metrics | 系统是否变慢、错误是否变多、容量是否接近上限？ | 带低基数标签的时间序列 | Prometheus 拉取 `/metrics`，计算错误率、延迟分位数、SLO 和告警 |
| Trace | 一次请求具体在哪一层耗时或失败？ | 一棵由 Trace/Span 组成的调用树 | OpenTelemetry 生成 Span，Jaeger 保存并提供 UI 查询 |
| Logs | 这次失败发生时，具体记录了什么上下文？ | 结构化事件 | `trace_id`/`span_id` 关联指标和 Trace，保留错误摘要与审计信息 |

Jaeger 是分布式链路追踪系统。它接收 OpenTelemetry 导出的 Span，按 `trace_id` 把一次请求的 HTTP、Agent、工具、Redis、MySQL、ES 等子操作拼成调用树，并在 `http://localhost:16686` 展示。它不是 Prometheus 的替代品，也不负责抓取指标；Prometheus 关注“趋势和告警”，Jaeger 关注“一次请求的定位”。本地 Compose 中 Jaeger 的 OTLP gRPC 接收端是 `4317`。

## 2. 当前进度判定

| 范围 | 当前事实 | 面试表述 |
| --- | --- | --- |
| Stage 01 | 配置、启动依赖检查、live/ready、优雅关闭已存在；S 级手写和 Compose 收尾仍有待办 | “启动和健康边界已经有代码与单测，部分训练交付尚未收尾” |
| Stage 02 | Redis 短期窗口、MySQL 历史、读回源和写入策略已有实现与单测；真实 Compose 集成和并发顺序待补 | “双层记忆链路已实现，生产并发语义仍需验收” |
| Stage 03 | 同步 Chat、SSE、取消传播、统一错误映射已实现；真实模型取消边界待复核 | “传输层闭环完成，真实供应商流仍需集成验证” |
| Stage 04 | ES BM25 + kNN + RRF、Mock Embedder、导入/检索基础已实现；版本、召回评估和部分集成仍在补 | “检索基线可运行，质量与生命周期治理继续补齐” |
| Stage 05 | Eino Agent、`health_check`、`knowledge_search`、手写 Loop 基准、多 Agent 编排和 ToolCall 审计已接通 | “Agent 的策略、工具边界、失败回退和审计已有生产化骨架” |
| Stage 06 | Compose 已声明 Prometheus/Jaeger；根模块没有 `/metrics`、SDK provider、HTTP middleware、Trace 注入和观测查询闭环 | “观测基础设施在部署文件中，应用侧接线是当前最高优先级缺口” |
| Stage 07-09 | 测试交付、面试交付、gRPC 演练规格已有，真实验收/拆分未完成 | “先完成单体可观测闭环，再谈服务拆分” |

证据：`cmd/pilot/main.go` 只注册健康、Chat、Agent 和知识入口；`manifest/docker-compose.yml` 已有两个观测容器；`manifest/prometheus/prometheus.yml` 已尝试抓取 `host.docker.internal:8080`，但当前应用没有对应的 `/metrics`。

## 3. 目标数据流

```text
HTTP 请求
  -> Request middleware：request_id / trace context / route 归一化
  -> Handler -> Chat/Agent Service
  -> Agent span -> Tool span -> Redis/MySQL/ES/LLM spans
  -> response middleware：status、duration、error、inflight
  -> Prometheus scrape /metrics
  -> OTLP exporter -> Jaeger :4317 -> Jaeger UI :16686
  -> JSON log：trace_id、span_id、request_id、error_code
```

一条排障闭环必须能从告警开始：

```text
Prometheus 告警（错误率/延迟/SLO）
  -> 取告警时间窗和 service/route
  -> Jaeger 按时间或 trace_id 定位慢 Span
  -> 结构化日志确认错误码和依赖响应
  -> 修复或回滚
  -> 回归测试 + 指标恢复 + 故障记录
```

## 4. 应用侧契约

### 4.1 包边界

新增 `internal/observability/`，至少包含：

- `provider.go`：初始化 MeterProvider、TracerProvider、OTLP exporter，暴露 `Shutdown(ctx)`。
- `middleware.go`：HTTP root span、请求计时、状态码记录、Trace-ID 响应头。
- `metrics.go`：所有业务指标定义与低基数标签约束。
- `attributes.go`：统一脱敏和属性白名单。

`cmd/pilot/wiring.go` 只负责装配 provider 和 middleware；Handler、Chat、Agent、Memory、Retriever 只依赖项目自己的观测接口或 `context.Context`，不把 exporter 细节散落到业务包。

### 4.2 依赖与配置

建议加入以下依赖，版本以 `go mod tidy` 解析的兼容版本为准：

- `github.com/prometheus/client_golang/prometheus` 与 `promhttp`；
- `go.opentelemetry.io/otel/sdk`；
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`；
- `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`（可选，若自定义中间件已覆盖则不重复包裹）。

建议配置：

```dotenv
OBS_ENABLED=true
OBS_METRICS_ADDR=:9091
OTEL_SERVICE_NAME=pilot
OTEL_RESOURCE_ATTRIBUTES=deployment.environment=local
OTEL_EXPORTER_OTLP_ENDPOINT=127.0.0.1:4317
OTEL_EXPORTER_OTLP_INSECURE=true
OTEL_TRACES_SAMPLER=parentbased_traceidratio
OTEL_TRACES_SAMPLER_ARG=1.0
OTEL_EXPORT_TIMEOUT=5s
```

本地使用 1.0 采样便于演示；生产建议 parent-based ratio，并按环境降低采样率。观测 exporter 失败时默认只记录一次限频告警，不阻塞主请求；`OBS_ENABLED=false` 时业务仍可启动，但 `/api/v1/health/ready` 的 `degraded` 必须说明 `observability`。

### 4.3 Metrics 契约

指标名称使用 `pilot_` 前缀，标签必须是固定枚举或归一化路由：

| 指标 | 类型 | 标签 | 用途 |
| --- | --- | --- | --- |
| `pilot_http_requests_total` | Counter | `method, route, status_class` | 请求量和错误率 |
| `pilot_http_request_duration_seconds` | Histogram | `method, route` | p50/p95/p99 延迟 |
| `pilot_http_inflight_requests` | Gauge | 无或 `route` | 并发压力 |
| `pilot_chat_turns_total` | Counter | `mode, result` | 同步/SSE 成功、取消、失败 |
| `pilot_agent_steps_total` | Counter | `result` | Agent 轮数与停止原因 |
| `pilot_tool_calls_total` | Counter | `tool, result` | 工具成功、失败、拒绝 |
| `pilot_dependency_requests_total` | Counter | `dependency, operation, result` | Redis/MySQL/ES/LLM 依赖失败 |
| `pilot_dependency_duration_seconds` | Histogram | `dependency, operation` | 依赖延迟 |
| `pilot_llm_tokens_total` | Counter | `direction, model` | token 成本；model 必须来自配置白名单 |

禁止把 `user_id`、`session_id`、完整 URL、query、trace_id、错误文本放进 Prometheus label；这些值只进入脱敏日志或 Trace event。路由必须使用 `/api/v1/chat` 这类模板值，不能使用实际 path。

### 4.4 Trace 契约

每个请求至少生成以下 Span：

1. `http.request`：`http.method`、归一化 `http.route`、状态码、结果类别。
2. `chat.generate` 或 `chat.stream`：`session.id` 只能在允许的内部调试环境记录，默认记录哈希或不记录；记录 `mode`、取消/超时结果。
3. `agent.run`、`agent.step`、`tool.<name>`：记录 step、工具结果、耗时和限制原因。
4. `memory.redis`、`memory.mysql`、`retriever.elasticsearch`、`llm.generate`：记录操作名、结果、超时，不记录 prompt、API key 或完整用户输入。

子调用只从上层 `context.Context` 派生 deadline，禁止重新创建 `context.Background()`。所有 Span 在错误时 `RecordError` 并设置 Error status；客户端取消要标记 `cancelled`，不要伪装成上游 500。

### 4.5 日志关联

统一使用 `slog` JSON handler，至少包含 `time`、`level`、`service`、`trace_id`、`span_id`、`request_id`、`route`、`error_code`。日志正文只能记录经过净化的错误摘要。用户 query、模型 prompt、密钥和完整 DSN 默认禁止写入日志；需要调试时使用受控采样和脱敏开关。

## 5. 端点和 Compose 验收

应用主 HTTP 端口保留 `8080`，新增：

- `GET /metrics`：Prometheus text exposition，健康接口和 metrics 端点不要求业务 API Key；
- `GET /api/v1/health/live`：只证明进程存活；
- `GET /api/v1/health/ready`：Redis、MySQL、ES 是核心依赖；观测 exporter 是可选依赖，失败时返回 degraded 语义。

推荐先用同一 HTTP server 暴露 `/metrics`，只有需要隔离凭证或高流量时才单独监听 `OBS_METRICS_ADDR`。Compose 中 Jaeger 保留 `16686` 和 `4317`；Prometheus 保留 `9090`，scrape target 必须与 Pilot 实际监听地址一致。

## 6. 分阶段实施

### P0：应用能被看见

1. 增加 Prometheus registry、`/metrics`、HTTP request counter/histogram/inflight。
2. 增加 OTel SDK + OTLP gRPC exporter，设置 service name/resource，应用退出时 flush。
3. 增加 HTTP root span、响应 `traceparent`/`X-Trace-ID`，把 trace/span ID 放进 slog。
4. 为 Chat、SSE、Agent、工具、Redis/MySQL/ES 增加最小 span。

验收：`curl http://localhost:8080/metrics` 能看到 Pilot 指标；请求一次 Chat 后，Jaeger UI 能按 service=`pilot` 查到完整 Trace。

### P1：故障可定位

1. 统一结果标签和错误码映射；补 Tool、Agent、依赖失败计数。
2. 增加 Prometheus recording rules 和告警：HTTP 5xx、p95、ready 失败、依赖错误、SSE 取消异常增长。
3. 增加 Grafana dashboard（请求量、错误率、p95、Agent 工具失败、依赖延迟、token 成本）。
4. 增加故障演练：停 ES、停 Redis、OTLP exporter 不可达、客户端中断 SSE。

### P2：企业化治理

1. 采样、队列上限、批量导出、shutdown flush deadline，防止观测拖垮业务。
2. 指标 cardinality review、日志留存与脱敏、Trace 访问控制和数据保留策略。
3. CI 运行单测、`go vet`、Compose smoke、指标断言、Trace 至少存在 root span 的集成测试。
4. 把告警、Runbook、修复 PR、回归测试链接到同一故障记录，形成可审计闭环。

## 7. 可复制验收命令

实现完成后按下面顺序执行，并把输出保存到阶段记录：

```bash
docker compose -f manifest/docker-compose.yml up -d redis mysql elasticsearch prometheus jaeger
go run ./cmd/pilot
curl -sf http://localhost:8080/api/v1/health/live
curl -sf http://localhost:8080/api/v1/health/ready
curl -s http://localhost:8080/metrics | rg 'pilot_http_requests_total|pilot_http_request_duration_seconds'
curl -s localhost:8080/api/v1/chat -H 'Content-Type: application/json' \
  -d '{"user_id":"obs-demo","session_id":"obs-demo-1","query":"检查 Redis 健康状态"}'
curl -sf http://localhost:9090/-/ready
curl -sf http://localhost:16686/api/services
```

然后打开 `http://localhost:16686`，选择 `pilot` 服务，确认一条 Trace 至少包含 `http.request` 和一个业务子 Span。再停止 Elasticsearch：

```bash
docker compose -f manifest/docker-compose.yml stop elasticsearch
curl -i http://localhost:8080/api/v1/health/ready
curl -s http://localhost:8080/metrics | rg 'dependency|http_requests_total'
docker compose -f manifest/docker-compose.yml start elasticsearch
```

预期是 ready 返回 503，业务错误带稳定错误码，Prometheus 错误计数增加，Jaeger 出现带 Error status 的依赖 Span；恢复后 ready 回到 200。当前仓库尚不能通过这些验收，因为 P0 的应用接线尚未实现。

## 8. Definition of Done

- [ ] `/metrics` 可抓取，指标名称、类型和标签通过 review。
- [ ] Chat、SSE、Agent、Tool、Redis、MySQL、ES 至少各有一个可查询 Span。
- [ ] Jaeger UI 能按 service、operation、错误状态筛选，并能从响应 Trace-ID 反查。
- [ ] 结构化日志含 trace/span 关联字段，且无密钥、DSN、完整 prompt。
- [ ] exporter 不可用不阻塞业务；ready 能表达 degraded；shutdown 在 deadline 内 flush。
- [ ] Prometheus 告警、Runbook、故障演练和回归测试齐全。
- [ ] 面试口播能用一条真实 curl 说明“告警 -> Trace -> 日志 -> 修复 -> 验证”。

## 9. 面试表达边界

当前阶段应说：“我已经把观测闭环拆成指标、Trace、日志三层，Compose 已准备 Prometheus 和 Jaeger，下一步按 P0 接应用 middleware 和 OTLP exporter；当前仓库还不能声称已经从 Jaeger 查到生产 Trace。”完成 P0 后再说：“一次 Chat 请求会从 HTTP root span 串到 Agent、工具和依赖，Prometheus 用于趋势告警，Jaeger 用于单请求定位。”
