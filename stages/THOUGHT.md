# 训练思考记录

> 本文件记录我们的阶段实训思考（与 `docs/review/THOUGHT.md` 课程资料分离）。

## Stage 01：工程骨架（设计复盘与手写待办）

### 本阶段目标

- 从文档规格出发建立可维护的 Go 服务骨架：配置、健康检查、依赖检查、优雅关闭。
- 设计依据：`docs/build/implementation/spec/01-environment.md` 与 `docs/build/stages/stage-01-skeleton.md`。

### 设计方案

- 目录按约定分层：`pkg`（可复用：envloader/configutil）、`internal`（业务私有：deps/controller）、`cmd/pilot`（组装根）。
- 入口编排：`envloader.Load(.env)` → `configutil.Load()` → `deps.CheckAll`（失败即退出）→ 注册路由 → `ListenAndServe` → 信号优雅关闭。
- 健康检查语义：`/live` 只证明进程存在；`/ready` 逐项报告 Redis/MySQL/ES；任一失败 503。
- 错误策略：核心依赖启动期强检查，错误只含依赖名，不泄露密码或完整 DSN。

### 字段建模评审（Config 配置结构）

总分：87/100（补评：编码前未按流程先让学习者提交字段设计，本轮补录）

| 维度 | 得分 | 评价 |
| --- | ---: | --- |
| 业务完整性 | 18/20 | 覆盖 `.env.example` 全部字段、分组合理；扣 2：`App.APIKey` 与 `LLM.APIKey` 语义相近易混 |
| 类型准确性 | 17/20 | bool/duration/int 用对；扣 3：TTL/Window 经 `ParseDuration` 失败时静默回退默认值，缺显式报错 |
| 约束与可空性 | 13/15 | 缺 `Validate()`（addr 非空、`EMBEDDING_DIM>0`、DSN 非空）；扣 2 |
| 查询与索引意识 | 14/15 | 本阶段无查询，折算为"默认值/降级意识"：默认值与 spec 一致；扣 1：`ALLOW_DEGRADED` 尚未被代码消费 |
| 状态与生命周期 | 8/10 | `Load()` 返回不可变值、无 setter；扣 2：无"必填/可缺省"显式标记 |
| 命名与可读性 | 9/10 | 字段名与 env key 一一对应；扣 1：`OTLPExportEndpoint` 略冗余 |
| 扩展与演进 | 8/10 | 分组结构利于 Stage 02+ 叠加；扣 2：无版本/来源标记 |

第一版主要问题：
- 配置解析错误（非法数字/时长）被静默吞掉，回退默认值，排障时会困惑。
- `ALLOW_DEGRADED` 只定义了语义，代码未消费（观测依赖降级留给 Stage 06）。

改进方向（学习者可选做）：
- 给 `configutil` 加 `Validate()`，错误显式返回而不是静默回退。

### 模块拆分理由

- `pkg/envloader`：只做"文件 → 环境变量"，无业务、可复用、可单测。
- `pkg/configutil`：纯映射（env → 结构），无 IO 无状态，是"配置单一来源"心智的落点。
- `internal/deps`：失败边界。负责聚合检查与错误净化，S 级核心。
- `internal/controller`：HTTP 契约层，健康检查 + 统一错误体。
- `cmd/pilot/main.go`：组装根（bootstrap），只做编排，不含业务。

### 数据流链路

1. 入口：`go run ./cmd/pilot` → `envloader.Load(".env")`（缺失静默）。
2. 校验：`configutil.Load()` 兜底默认值；强校验缺失（见局限）。
3. 业务处理：启动期 `deps.CheckAll`（redis PING / mysql PingContext / es HTTP GET），10s 超时聚合。
4. 存储/外部调用：三依赖仅连通性探测，无业务读写。
5. 返回/副作用：HTTP `:8080`；`/live` 200；`/ready` 200/503。
6. 失败路径：核心依赖 down → 聚合错误（只含依赖名）→ exit 1；运行期 down → `/ready` 503；信号 → Shutdown 10s 宽限。

### 代码分级

| 模块/函数 | 等级 | 原因 | 学习者动作 |
| --- | --- | --- | --- |
| `cmd/pilot` 优雅关闭 | S | 生命周期边界，错则丢请求/泄漏 | 路线B：删除参考实现重写 |
| `internal/deps` CheckAll + 错误净化 | S | 失败聚合 + 凭据安全 | 路线B：删除参考实现重写 |
| `internal/controller` Ready + 错误映射 | S | 对外契约 | 路线B：删除参考实现重写 |
| `pkg/configutil` Config 结构 | A | DTO/配置结构 | 重构 + 补字段选型理由 + 可选 Validate |
| `pkg/envloader` | A | 配置加载 | 已沉淀拆解文档（`stage-01/`），能讲清即可保留 |
| `internal/controller` Live/WriteError | B | 简单脚手架 | 读懂复用 |
| Makefile / compose / `.env.example` | B | 环境脚手架 | 读懂复用 |

### 设计模式取舍

- 手动注入（`NewHealth(checkers)`）：采用。收益：controller 可测；代价：无容器，装配在 main 手写，Stage 02+ 依赖变多时再评估。
- 函数字段替代接口（`Dependency{Name, Check func}`）：采用。收益：构造与测试简单；代价：无状态、扩展性弱，接入正式 client 后再抽接口。
- 第三方 router/logger：不采用。`net/http` ServeMux + `slog` 满足阶段目标；观测中间件阶段再评估。

### 踩坑与边界

- MySQL driver 必须 blank import，否则 `sql.Open` 报 unknown driver（曾把 open 错误误判为 invalid DSN）。
- mysql driver 的 open 错误可能含 DSN 字符串 → 必须掩码。
- go-redis 默认 logger 在依赖不可用时刷屏 → 用 `silentLogger` 抑制。
- Shutdown 不能用被信号取消的 ctx（会立即超时失效）。
- 本机已有 mysqld 占用 3306，`compose up` 前需处理端口冲突。

## Stage 02：Redis 短期记忆与 MySQL 历史（当前思考）

### 当前设计

- `user_id + session_id` 标识一个会话。
- MySQL 保存每条消息的一行，是完整历史的事实来源。
- Redis 使用一个会话 key 对应一个 List；List 中每个元素是独立 JSON 消息，不是把整段历史序列化成一个数组值。
- `limit` 保留最新消息窗口，`ttl` 控制窗口过期时间。
- `recentMemoryKey` 只构造会话容器地址，不包含消息内容；消息内容保存在该 key 对应的 List 中。

### Redis 写入时序

```text
校验整批消息
  -> 确认 user_id/session_id 一致
  -> 补 CreatedAt 并 JSON 序列化
  -> MULTI/EXEC
       -> RPUSH 批量追加
       -> LTRIM 保留最新 N 条
       -> EXPIRE 刷新 TTL
```

Redis 的 `MULTI/EXEC` 不是 MySQL 式的失败回滚事务。这里的目的，是让追加、裁剪和 TTL 刷新作为一个 Redis 命令批次执行，避免其他请求看到中间状态；命令参数应在进入事务前全部校验和序列化。

### Context 边界

- HTTP 控制层或 service 层拥有请求生命周期，负责把 `r.Context()` 传入存储层。
- Redis/MySQL adapter 不创建脱离请求的 `context.Background()`，否则客户端断开后底层操作仍可能继续。
- 存储层只有在需要更短的本地超时时，才基于传入 ctx 使用 `context.WithTimeout`，并负责 `cancel()`；不能覆盖调用方更早的取消信号。
- 后台补偿或异步重建窗口是另一条生命周期，不能复用已结束的请求 ctx，应在明确的 worker 生命周期中创建新的 ctx。

### Service 协调层

- 文件位于 `internal/memory/service.go`，继续使用 `package memory`；当前没有必要拆出 `internal/memory/service/` 子包。
- `Service` 只依赖 `HistoryStore` 和 `RecentMemory`，不直接依赖 SQL 或 Redis 客户端。
- `LoadRecent` 的顺序是 Redis 命中直接返回；空结果回源 MySQL；MySQL 有结果时尝试回填 Redis。
- `SaveMessages` 先写 MySQL，再写 Redis。MySQL 是持久事实来源，Redis 更新失败只记录告警，不撤销已成功的持久化。
- `user_id` 和 `session_id` 是显式业务参数，不放进 `context.Context`；Context 只负责取消、超时和请求范围生命周期。

### Service 数据流

```text
Controller / 上层用例
  -> Service.LoadRecent
     -> Redis 命中：返回
     -> Redis 未命中：MySQL.List -> 尝试 Redis.Append 回填

Controller / 上层用例
  -> Service.SaveMessages
     -> 校验整批消息
     -> MySQL.Append
     -> Redis.Append
```

### 代码分级补充

| 模块/函数 | 等级 | 学习者动作 |
| --- | --- | --- |
| `memory.Service.LoadRecent` | S | 能解释缓存命中、回源、回填和失败策略 |
| `memory.Service.SaveMessages` | S | 能解释持久化优先级、批次校验和缓存失败边界 |
| `fakeHistoryStore` / `fakeRecentMemory` | A | 阅读测试替身，能自己增加边界用例 |

### 已完成测试与剩余问题

- 已完成：Redis key 隔离、批次会话一致性、Service 缓存命中/回源、MySQL 失败阻断 Redis、Redis 失败不影响历史返回。
- 已完成：MySQL 事务写入、Redis List 窗口裁剪、TTL 刷新和零值 `CreatedAt` 处理。
- 待补：真实 MySQL/Redis Compose 集成验证，以及同一会话并发写入时的顺序策略。
- 需要决定 Stage 03 流式响应中 user 与 assistant 是否分两次追加；这不应强行要求和同步请求一样的批量边界。

### 复盘思考题（学习者回答）

1. 这条数据流从入口到返回经历了哪些层？
2. 哪段逻辑是 S 级？为什么必须手写？
3. 配置字段的类型（duration/bool/int/string）你为什么这么选？
4. 哪些配置影响启动成败？哪些可以缺省？
5. 当前设计最大局限是什么？
6. 如果 `/ready` 请求变多，哪里最先出问题（每请求 3s 串行 ping 三依赖）？
7. 面试官问"为什么这样分层（pkg/internal/cmd）"，你怎么回答？
8. 为什么 `SaveMessages` 先写 MySQL 再写 Redis？如果 Redis 失败，为什么不返回整个请求失败？
9. 哪些地方应该透传 `context.Context`，哪些地方才需要创建新的 Context？

（S 级手写完成后在此补充回答，见 `stage-01/s-level-handwriting-guide.md`）

### 面试口述素材（待填写）

- 项目背景：
- 我的职责：
- 核心难点：
- 方案取舍：
- 结果与优化：

## Stage 05：Agent 策略、中间件与并发边界（当前思考）

### 问题背景

Agent 的一次模型响应可能包含多个 Tool Call。Eino 当前配置为顺序执行，但策略层不能依赖这个实现细节；未来开启并行工具调用后，多个 goroutine 会同时访问本次运行的预算、重复失败计数和审计状态。

### `runState` 与临界区

本次 Agent 执行必须创建独立的 `runState`，通过当前请求的 `context.Context` 传给工具中间件，不能放在 Agent 实例字段或全局变量中，否则并发请求会互相污染。状态至少包括：

- `toolCalls`：本次请求已接受的工具调用数；
- `consecutiveFails`：工具名和参数指纹对应的连续失败次数；
- 审计记录和限制原因。

`toolCalls++` 是读-改-写操作，`map` 也不能并发读写，因此访问这些字段必须加锁。锁只保护内存状态：

```text
加锁 -> 检查预算并递增 -> 解锁
执行 next（网络/磁盘/数据库 IO，不持锁）
加锁 -> 更新成功或失败计数 -> 解锁
```

如果把锁持有到 `next` 返回，多个并行工具会被强制串行，策略中间件反而破坏了并行能力。可以带走的并发原则是：**锁保护共享内存，不保护耗时 IO**。

### 参数指纹

重复调用检测使用 `工具名 + 规范化 JSON 参数` 生成 SHA-256 指纹，不包含模型生成的 CallID。不能只挑几个“关键字段”，否则不同参数可能被误判为相同，也会让通用中间件依赖具体工具。

规范化规则需要固定：先校验 JSON，再按稳定的对象键顺序重新编码；数组顺序仍然有业务意义，不能随意排序。指纹只用于本次 Run 内的重复失败保护，不作为跨请求的幂等键。

### 策略拒绝与基础设施错误

两者必须分开，不能都叫 `tool execution failed`：

| 类型 | 例子 | 是否执行真实工具 | 审计状态 | 对模型/Runner 的语义 |
| --- | --- | --- | --- | --- |
| AI Policy Error | 参数过大、工具不在白名单、调用预算耗尽、重复失败达到阈值 | 否 | `rejected` | 返回稳定的结构化 Tool Observation；默认不取消整个 Context |
| Infrastructure Error | Redis/ES 超时、连接拒绝、依赖返回 5xx | 是 | `failed` | 保留可重试/不可重试语义，交给 fallback 或有限重试处理 |
| Model/Runner Error | 模型调用失败、Graph 超时、上下文取消 | 不适用 | 保留已有审计 | 结束本次 Agent，返回稳定错误或有限答案 |

参数超限采用软拒绝：中间件不调用 `next`，返回结构化错误结果且不主动 `cancel`。这样模型有机会修正参数，Eino 的其他节点也不会因为一个普通策略判断被粗暴打断。但必须配合重复拒绝阈值，否则模型可能无限修正或重复调用。

基础设施错误不能被伪装成“没有知识”或“策略拒绝”。是否把它转换成正常 Tool Observation，还是直接让 Eino 返回错误，需要通过集成测试确认；无论采用哪种方式，审计都应记录为 `failed`，并保留稳定的依赖错误码。

### 设计边界与实施顺序

1. `internal/agent`：只保存 `BudgetPolicy`、`RepeatCallPolicy`、`FallbackPolicy` 和校验逻辑，不导入 Eino。
2. `internal/ai/eino`：用 `compose.ToolMiddleware` 实现参数检查、调用预算、重复失败保护和审计切面；普通 Tool 代码不感知策略。
3. `PolicyAwareRunner`：在 Run 前派生超时 Context、创建 `runState`，Run 后汇总审计和 Fallback。它不重新实现 Eino Loop。
4. 先用 fake tool 和并发测试验证临界区，再做 Eino 两轮调用和软拒绝集成测试。

### 待确认与待验证

- Eino 在 Tool Middleware 返回 `ToolOutput, nil` 时是否会稳定地把结果交回模型并继续下一轮；需要集成测试确认。
- Eino `Generate` 失败时是否能取得此前的 assistant 部分答案，决定 `FallbackAnswerWithoutTool` 的可用范围。
- 当前 `ToolCallStatus` 只有 `succeeded/failed`，实现策略拒绝前需要增加 `rejected`，否则审计会丢失重要语义。

### 当前实现进展

- 已完成参数超限和工具调用总预算 Middleware：策略拒绝返回结构化 Observation，不取消 Agent Context。
- 已完成单次 Run 内的重复失败保护：使用工具名和规范化 JSON 参数的 SHA-256 指纹；成功调用清除该指纹的失败计数，达到阈值后后续调用标记为 `rejected`。
- 预算预占、失败计数和拒绝审计均由短粒度互斥锁保护，真实 Tool 的 `next` 执行不持锁；并发行为已通过 `go test -race` 验证。
- 这仍不是跨请求共享的 Circuit Breaker。跨请求熔断需要独立状态、半开探测和实例间协调，暂不纳入本切片。
- 已接入请求级错误收口：策略超时和上层取消分别映射为稳定的 `ErrAgentTimeout`/`ErrAgentCancelled`；普通 Eino 错误保留底层原因并写入 `limitations`。当前没有可靠的部分 assistant 文本，因此无文本时不会强行执行“无工具回答”回退。

### 三种执行粒度与 PolicyAwareRunner（已实现）

Middleware 的触发时机是**每次真实工具执行**，不是每轮模型调用：模型第一轮直接返回文本时中间件一次都不会执行；一轮返回 N 个 ToolCall（`ExecuteSequentially: true`）则顺序各过一次中间件。跨轮生效靠 `EinoRunner.Run` 每次请求创建一次 `policyRunState` 并随 `runCtx` 下发。

三种粒度的最终分工：

| 粒度 | 负责策略 | 执行位置 |
| --- | --- | --- |
| 工具调用级 | 参数上限、调用预算、重复失败保护 | Eino `ToolCallMiddlewares` |
| 模型轮次级 | MaxSteps、step 计数 | Eino ReAct 循环（MaxStep）+ audit 回调 |
| 请求级 | 总超时、超时/取消稳定语义、FallbackPolicy | `agent.PolicyAwareRunner` |

- `internal/agent/policy_runner.go` 新增请求级包装器：Run 前从请求 ctx 派生策略超时；失败时先判断生命周期（错误链包含 `DeadlineExceeded`/`Canceled`，或 `runCtx.Err()` 已到期——内部 Runner 可能把超时包进框架错误，两种都要看），超时/取消优先返回 `ErrAgentTimeout`/`ErrAgentCancelled`，不被普通工具错误掩盖；普通失败再按 `ResolveFallback` 收口。
- `AgentPolicy.ResolveFallback` 落在 contract 层：`answer_without_tool` 只在模型已产出非空文本时降级为有限回答，`return_error` 透传原始错误，两者都写入 limitations 并保留工具审计。
- 组合根装配顺序：`EinoRunner`（能力层）→ `PolicyAwareRunner`（请求级收口）。EinoRunner 内部仍保留一次同策略超时派生作为兜底；两层同时派生时先到者生效，语义一致。
- Handler 映射：`ErrAgentTimeout` → 504 `agent_timeout`，`ErrAgentCancelled` → 499 `agent_cancelled`，让客户端能区分“失败”和“还没算完”。
- 待验证（留给集成测试）：`FallbackAnswerWithoutTool` 在 Eino 路径下的可用范围——`Generate` 失败时能否拿到此前的 assistant 部分文本，决定降级回答是否真的存在。

### Stage 05 收尾验证记录（2026-09-08）

- `go test -tags integration ./internal/ai/retriever`：`TestESRetrieverEndToEnd`、`TestKnowledgeIngestorBulkThenRetrieve` 通过（真实 ES 8.18.8，含 `-race`）。
- RAG 评估链路端到端跑通：三篇语料（redis-timeout / mysql-replication / elasticsearch-cluster，EMBEDDING_DIM=32 mock）导入 `pilot_knowledge` 后执行 `make rag-eval`，基线报告存于 `reports/rag-eval.json`：
  - HitRate@5=1.00，Recall@5=1.00，Precision@5=0.20，MRR=0.96，nDCG@5=0.97，DuplicateRate@5=0.00，14 条查询 0 失败。
  - 注意：评估集期望的 `doc_id` 与语料文件名严格一致（如 `elasticsearch-cluster`），导入时的 doc_id 必须对齐，否则表现为"检回但判失败"。
  - 当前语料每篇只切出 1 个 chunk，Precision@5=0.20 主要是"语料太少、Top-5 全被同批文档占满"的必然结果；语料扩充后该指标才有调参意义。
- 环境限制：`LLM_MODE=mock` 时 Agent 不组装，`POST /api/v1/agent/chat` 返回 503 `agent_unavailable`；真实模型的全链路验证（health_check → knowledge_search → 最终回答）需要 API key，本地仅能由 scripted model 集成测试覆盖工具链路。
- 导入方式备忘：评估前需要把语料写入 ES；当前只能走 `POST /api/v1/knowledge/documents`（需全服务启动），后续计划给 `cmd/eval` 增加 `-import` 模式。

### 多代理编排增强：分级调度、断点续跑与微基准（2026-09-16）

压力面复盘（`面试经历/01`）暴露的三个差距在同一切片内补齐：

- **复杂度分级调度**（`planner.go`）：`PlanResult` 新增 `Complexity`（simple/complex）。规则层用级联关键词（集群/大面积/雪崩/连环）打标，LLM 分类在同一个 JSON 里输出；解析缺失或非法一律按 complex（安全侧）。路由变化：simple 排障只跑知识分支（低精度先行），complex/未知意图双分支全跑。这是"意图分诊裁剪 + 复杂度分级裁剪"的第二级。
- **断点续跑**（`taskstate.go` + `orchestrator.go`）：分支汇合后、综合前写任务快照（Redis JSON，TTL 5 分钟，`DefaultSnapshotTTL`）。快照带状态机（running/succeeded/failed）与 `StartedAt` 时间戳：只有 running 且未超窗口的快照允许续跑，终态快照主动删除（TTL 兜底），失败重试读旧状态的窗口被时间戳校验关死。续跑复用已完成分支的证据、只补跑缺失分支；快照层故障只降级记日志，不阻断请求——快照是恢复优化，不是正确性依赖。任务 ID 由会话+查询 SHA-256 派生，天然隔离。
- **微基准**（`cmd/bench`）：固定五场景 × 100 请求 × 20 并发，脚本模型 + 桩工具隔离 LLM 与外部依赖。报告 `reports/multiagent-bench.json`：编排开销亚毫秒（p50 0.04–0.10ms）；simple 扇出 1.0 vs complex 2.0；分支超时场景 limitation 100% 触发、0 请求失败（隔离有效）；分类超时场景按 3s 预算回退规则；引用缺失场景 100% 走确定性摘要。
- **mock 模式闭环**：`buildTeamAgent` 在 mock 模式换用 `multiagent.ScriptedModel`（确定性脚本模型，走完全相同的编排路径），`POST /api/v1/agent/team/chat` 本地可端到端验证。注意这与单代理不同：ReAct 需要真实 tool-calling 模型，mock 下仍不组装（503）。
- 已知边界：`ALLOW_DEGRADED` 配置已加载但主入口启动检查未消费，是既有欠账。
- HTTP 全链路已补验（2026-09-16，真实 LLM=doubao/ark + Compose 依赖）：happy path（simple 分级单分支 + 引用综合，零降级，端到端 ~17s）、分类 3s 超时回退规则、综合超时降级、引用校验真实拦截（5 次综合 2 次未带编号）、运行中快照写入与终态删除（key 后缀 == 响应 task_id）均实测通过。真实模型观察：doubao 分类与规则基线有分歧（明显故障判 general，安全侧双分支兜住）；检索质量受 mock embedder 限制。详见 `面试经历/02` §1。
