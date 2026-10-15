# Pilot 项目面经与闭环口播稿

以下内容只使用当前仓库能确认的事实。Prometheus、OpenTelemetry、Jaeger 的应用接线仍在 Stage 06 施工规格中，因此口播会明确说“Compose 已声明、应用侧待接线”，避免把规划冒充上线结果。

## 1. 项目简介

Pilot 是一个用 Go 构建的运维排障 AI 助手，提供同步聊天、SSE 流式聊天、Agent 工具编排、知识库混合检索和多代理排障。Redis 保存近期会话窗口，MySQL 保存长期历史，Elasticsearch 承载知识检索；当前仓库已经完成 Agent/RAG 的主要业务骨架和测试，Prometheus、OpenTelemetry、Jaeger 的部署规格已完成，应用侧观测接线是下一阶段重点。

## 2. 简历 bullet

- **分层请求编排：** 面向运维排障场景，将 HTTP、聊天服务、记忆、Agent、工具和检索拆成清晰边界，同步与 SSE 复用同一业务编排，结果是请求校验、错误映射、取消传播和持久化职责可以分别测试；下一步用统一 Trace 串起这些边界。
- **双层记忆一致性：** 针对近期上下文读取快、长期历史可靠保存的不同需求，采用 Redis 窗口加 MySQL 事实存储，Service 负责缓存命中、回源、回填和失败策略，结果是缓存故障不会伪装成历史事实丢失；真实 Compose 并发顺序仍待补测。
- **可控 Agent 执行：** 针对模型重复调用、越权工具和上游失败，建立工具白名单、step/调用预算、参数大小限制、重复失败停止和稳定回退，并保留 ToolCall 审计，结果是 Agent 有明确停止条件和可解释失败边界。
- **混合检索证据链：** 针对关键词和语义召回各有盲点，采用 ES BM25 与向量 kNN 两路召回、RRF 按排名融合、chunk 去重和来源标注，结果是模型上下文带有可追踪证据；召回质量继续通过离线评估验证。
- **可观测性工程化：** 针对“知道失败但不知道慢在哪一层”的排障问题，设计 Prometheus 指标、OpenTelemetry Span、结构化日志和 Jaeger 查询的闭环，明确低基数标签、采样、脱敏和故障演练；当前 Compose 已声明组件，应用侧 `/metrics` 与 OTLP exporter 待实施。

## 3. 面试问题与第一人称口播

### A. 项目全貌与进度

#### 主问 1：请你介绍一下 Pilot 项目。

**口播：** 我做的是一个面向运维排障的 Go AI 助手。用户可以通过同步接口或 SSE 流式接口提问，系统先读取 Redis 中的近期会话，再按场景进入普通聊天或 Agent 工具编排；Agent 可以调用健康检查和知识检索，完整历史写入 MySQL，知识文档落到 Elasticsearch。当前我已经把聊天、记忆、RAG、Agent、工具审计和多代理骨架接通，并且有单元测试和部分集成测试。可观测性方面，Compose 已经准备 Prometheus 和 Jaeger，但应用侧的 metrics endpoint、OpenTelemetry provider 和 Trace middleware 还在 Stage 06 施工，所以我会把这部分明确说成下一步闭环，而不会把容器声明说成已经上线。

**追问 1：你的个人职责边界是什么？**

**口播：** 我把个人职责限定在代码和设计中能被仓库证据支持的部分：我负责梳理 HTTP 到业务服务的分层、Redis 和 MySQL 的记忆策略、SSE 的事件生命周期、RAG 的 RRF 融合、Agent 的策略和工具注册，以及对应的测试和施工规格。对于 Prometheus 和 Jaeger，我目前完成了数据模型、指标标签、Trace 层级、失败策略和验收命令的设计，应用接线还没有完成，因此不会把“已经查到 Jaeger Trace”作为结果。这样回答的好处是边界清楚，面试官追问时我能直接指向文件、测试和待办，而不是依靠无法验证的口头数字。

**追问 2：你如何证明项目不是只有 Demo？**

**口播：** 我会从失败路径证明它有工程约束。请求体有大小和未知字段限制，Redis 和 MySQL 的职责被 Service 隔离，SSE 客户端断开会向下游传播 context，Agent 有 step、工具调用和参数预算，工具失败会进入稳定回退，RAG 的两路结果会去重并保留来源。测试覆盖了这些边界，`go test ./...` 当前是通过的。对于生产化还缺少 Compose 下的完整冒烟和观测闭环，所以我的结论是核心业务骨架可验证，企业级交付仍按 Stage 06、07 的验收门槛推进。

#### 主问 2：目前项目完成到什么程度？

**口播：** 我会按阶段而不是按文件数量回答。Stage 01 的配置、启动依赖检查、live/ready 和优雅关闭已经有实现；Stage 02 的 Redis 短期窗口和 MySQL 长期历史已经有 Service 与测试，但真实 Compose 并发顺序还要补；Stage 03 的同步 Chat、SSE、取消传播和错误映射已经接通；Stage 04 的 ES BM25、向量 kNN、RRF 和 Mock Embedder 已有基础；Stage 05 的 Eino Agent、health_check、knowledge_search、手写 Loop 基准、多代理编排和 ToolCall 审计已接入。Stage 06 当前最大的缺口是应用没有 `/metrics` 和 OTLP exporter，Stage 07 到 09 主要还是规格和待验收工作。

**追问 1：为什么 README 里会出现“已完成”和“待补”同时存在？**

**口播：** 因为这里把“代码能力完成”和“生产验收完成”分开记录。例如 RAG 的 RRF 单测和带真实 ES 的基础集成已经通过，但文档版本、召回评估、并发和长时间运行还没有全部收尾；同样，Compose 声明了观测容器，但应用侧没有发指标和 Span。面试时我会给出这种分层状态：代码路径是什么、测试证据是什么、仍缺哪条验收证据。这样既能说明进展，也能说明我知道上线标准不只是把依赖写进 YAML。

**追问 2：下一步你先做什么，为什么？**

**口播：** 我会先做 Stage 06 的 P0，因为当前业务已经有 Chat、Agent、Tool、Redis、MySQL 和 ES 这些值得观测的节点，但没有统一入口看它们。第一步加低基数 HTTP metrics 和 `/metrics`，第二步初始化 OpenTelemetry tracer provider，把 root span 和 Trace-ID 接到日志，第三步为 Agent、工具和依赖调用补子 Span，最后用一次真实请求在 Prometheus 和 Jaeger 两边验收。只有这条链路可查询，后续的告警、Dashboard、故障演练和服务拆分才有证据基础。

#### 主问 3：为什么先做单体，而不是一开始就拆微服务？

**口播：** 当前主要风险是业务边界和失败语义还在收敛，而不是单机吞吐已经证明不够。先保持单体，可以避免网络超时、跨服务重试、分布式 Trace 和数据一致性把学习重点分散；同时我仍然在代码层保持 Handler、Agent、Memory、Retriever 和 Observability 的接口边界。等聊天、检索和观测的输入输出稳定后，我会优先演练拆知识检索或日志查询，因为它们资源模型相对独立。拆分前先定义 protobuf、deadline、错误码、Trace 透传和数据归属，再比较拆分收益，而不是为了使用 gRPC 而拆分。

**追问 1：单体会不会导致模块边界腐化？**

**口播：** 会有这个风险，所以我用组合根和项目契约控制边界。`cmd/pilot` 负责装配，Handler 只做协议转换，chat Service 负责用例，Agent 依赖项目自己的 Runner 和 Tool 接口，Eino 只在适配层出现，记忆和检索通过端口隔离。评审时我会检查业务包是否直接 import 供应商类型、是否把存储调用散落在 Controller，以及 context 是否被重新创建。单体只是部署形态，不代表所有代码都可以互相调用；边界先稳定，未来拆分才有成本可控的基础。

**追问 2：什么时候你会认为必须拆？**

**口播：** 我会看可观测数据和容量边界，而不是凭感觉。比如检索请求的 CPU、内存、索引访问模式和发布节奏已经明显独立，或者 Agent 主链路被检索故障拖慢，导致需要不同的扩缩容和隔离策略，这时才有拆分依据。前提是 Prometheus 已经能提供按依赖和路由的延迟、错误、并发数据，Jaeger 能显示跨边界调用，故障演练能验证超时和重试。没有这些证据时拆服务只会增加排障面，并不能自动提升企业级质量。

### B. Prometheus、Trace 与 Jaeger

#### 主问 4：Prometheus 和 Jaeger 分别解决什么问题？

**口播：** Prometheus 主要回答系统整体趋势问题，例如每分钟请求量、5xx 比例、p95 延迟、ready 失败次数和依赖错误是否持续上升。它通过抓取 `/metrics` 保存带标签的时间序列，适合 recording rule、SLO 和告警。Jaeger 主要回答单次请求的路径问题，一次请求会有一个 Trace ID，下面挂 HTTP、Agent、工具、Redis、MySQL 和 ES 等 Span，每个 Span 有开始时间、耗时、父子关系和错误状态。我的使用方式是 Prometheus 先发现“某个路由变慢”，Jaeger 再定位“这一条请求到底卡在 ES 还是模型”。两者配合日志中的 trace_id 才形成完整排障链路。

**追问 1：Jaeger 是不是日志系统？**

**口播：** 不是。Jaeger 存储和展示的是 Trace/Span 结构，重点是跨组件的父子关系、时间线和错误状态；日志是离散事件，适合记录错误码、依赖响应摘要和审计字段。一次请求可以有很多日志，也可以因为采样没有完整 Trace，但如果日志里带有 trace_id，就能在两者之间互相跳转。当前 Compose 的 Jaeger all-in-one 通过 OTLP gRPC 4317 接收 Span，UI 在 16686 查询；我不会把 Jaeger 说成 Prometheus 的指标存储，也不会用日志数量替代请求延迟指标。

**追问 2：为什么不用 Jaeger 直接做告警？**

**口播：** Trace 更适合详细定位，存储量大且通常有采样；如果用它直接做持续告警，会让告警成本、查询稳定性和统计口径都变复杂。Prometheus 的 Counter、Histogram 和 recording rule 更适合持续计算错误率、p95、SLO burn rate，再把告警时间窗和服务标签交给工程师。定位时再根据告警时间、route 和错误状态去 Jaeger 查代表性 Trace。这样职责边界清楚，也能在 Trace 采样率较低时仍然保留可靠的总量和趋势信号。

#### 主问 5：你现在为什么还看不到 Prometheus 和 Jaeger？

**口播：** 现在能看到的是基础设施容器，而不是 Pilot 的观测数据。Compose 已经声明 Prometheus 9090、Jaeger UI 16686 和 OTLP 4317，Prometheus 配置也尝试抓取 `host.docker.internal:8080`；但根模块的 `cmd/pilot` 只注册健康、聊天、Agent 和知识路由，没有 `/metrics`，代码也没有初始化 OpenTelemetry SDK、TracerProvider、OTLP exporter 或 HTTP middleware。因此即使容器是健康的，Prometheus 没有可抓的数据，Jaeger 也没有 Pilot 的 Span。我的 Stage 06 P0 规格已经把依赖、接口、指标、Span、验收命令和故障演练列出来，下一步是应用接线。

**追问 1：把容器加进 Compose 算不算完成监控？**

**口播：** 不算。容器只是运行时基础设施，闭环至少还要有应用暴露指标、Prometheus target 能抓到、一次业务请求产生指标样本、SDK 把 Span 发送给 Jaeger、Jaeger UI 能按 service 查到 Trace，并且故障时能看到错误状态。还要验证 exporter 不可用时不会阻塞主请求，指标标签不会产生用户级高基数。也就是说部署文件解决“服务在哪里”，应用接线解决“服务产生什么数据”，验收脚本和故障演练解决“数据能不能帮助修复问题”。

**追问 2：你会怎样快速证明接线成功？**

**口播：** 我会先启动 Redis、MySQL、ES、Prometheus 和 Jaeger，再启动 Pilot，调用 live、ready 和一次 Chat。随后请求 `/metrics`，确认 `pilot_http_requests_total` 和延迟 Histogram 有样本；访问 Prometheus 的 ready 接口，确认 target 没有 scrape error；访问 Jaeger 的 services API 或 UI，选择 `pilot`，确认有 `http.request` 以及一个业务子 Span。最后停掉 ES，再重复 ready 和 Chat，观察依赖错误指标增加、Jaeger 对应 Span 标记 Error，恢复 ES 后 ready 回到 200。这个过程同时验证可见性和故障定位。

#### 主问 6：你会怎样设计一条 Chat 的 Trace？

**口播：** 请求进入时先创建或提取 root span `http.request`，记录归一化路由、方法和最终状态。同步聊天进入 `chat.generate`，SSE 进入 `chat.stream`，二者都从同一个 request context 派生下游 deadline。读取近期记忆创建 Redis span，必要时回源 MySQL；Agent 场景创建 `agent.run` 和每轮 `agent.step`，每次工具调用创建 `tool.health_check` 或 `tool.knowledge_search`，检索再创建 ES span，模型调用创建 LLM span。所有错误调用 `RecordError` 并设置稳定状态，客户端取消标记 cancelled。日志只带 trace_id/span_id 和脱敏摘要，不能把 prompt、密钥和完整用户输入塞进 Span。

**追问 1：Trace 属性怎么避免泄露隐私？**

**口播：** 我会把属性分成白名单和禁止项。白名单包括 route 模板、HTTP method、status class、operation、dependency、result、timeout 和 tool 名称；user_id、session_id 默认不记录，确有内部调试需求时只记录不可逆哈希并受环境开关控制。完整 query、prompt、API key、DSN、模型原始错误和工具参数不进入 Span 或 Prometheus label，必要的错误摘要经过净化后进入结构化日志。这个边界既保护隐私，也避免 trace 和指标体积失控，属于可观测性设计的一部分，而不是上线后再补的安全检查。

**追问 2：为什么要把 context 一直透传？**

**口播：** 因为 Trace 的父子关系、deadline 和客户端取消都依赖同一个 context。如果某一层重新用 `context.Background()`，下游就会失去 trace parent，也可能在请求已经取消后继续调用模型、Redis 或 ES。我的规则是所有模型、工具和存储调用都接收上层 context，只能从它派生更短的 timeout；SSE 断开时，request context 取消，service stream、Agent runner 和工具执行一起停止。这样一次请求的时间边界和诊断上下文是同一条链，而不是每个模块各自计时。

### C. 指标、SLO 与故障治理

#### 主问 7：你会采集哪些指标？

**口播：** 我会先覆盖四类指标。第一类是 HTTP 请求总量、状态类别、延迟 Histogram 和 inflight，用来计算流量、错误率、p95 和容量。第二类是业务指标，例如 Chat 成功/取消/失败、Agent step 停止原因、工具成功/失败/拒绝。第三类是依赖指标，例如 Redis、MySQL、ES 和 LLM 的操作量、结果和耗时。第四类是成本指标，例如输入输出 token 总量。标签只使用 method、归一化 route、status class、固定的 tool/dependency/result，绝不把 user_id、session_id、trace_id、完整 URL 或 query 放进去。这样既能支持 SLO，也不会因为每个用户产生新时间序列。

**追问 1：为什么用 Histogram 而不是只记录平均耗时？**

**口播：** 平均值会掩盖长尾，尤其是 Agent、ES 和模型调用经常有少量很慢的请求。Histogram 把延迟落在预设桶里，Prometheus 可以计算 p50、p95、p99，进而判断大多数请求和长尾请求是否分别达标。桶边界要结合接口预算设计，比如普通 Chat、SSE 首 token、Agent 总耗时和依赖调用可以使用不同桶；不能无限增加细粒度标签。面试时我会说明这是对用户体验和容量风险的观测，而不是把一个平均数当成系统性能结论。

**追问 2：如何控制 Prometheus 的 cardinality？**

**口播：** 第一，路由使用 `/api/v1/chat` 这种模板值，不能把实际 path 当标签；第二，标签集合在代码中固定白名单，禁止用户输入、错误文本、trace_id 和 query；第三，model、tool、dependency 等值来自受控配置或注册表；第四，在 review 和测试中统计时间序列数量，发现异常增长就阻断合并。高维细节放进 Trace 或日志，且日志也要有采样和脱敏。这个分层原则是指标服务整体趋势、Trace 服务单次调用、日志服务事件细节。

#### 主问 8：你如何从指标定义 SLO 和告警？

**口播：** 我会先把用户可感知的 SLI 写出来，而不是从组件 CPU 开始。比如 Chat 请求成功率可以用非 5xx 请求占比，延迟用请求 Histogram 的 p95，ready 可用性用健康接口返回 200 的比例，Agent 工具则观察失败和超时占比。然后按环境设定目标和观察窗口，再用 recording rule 计算错误预算消耗，告警里带 service、route、dependency 和时间窗。告警触发后不直接下结论，而是到 Jaeger 查代表性 Trace，再结合结构化日志确认根因；修复后要跑回归测试并观察指标恢复，形成可审计故障记录。

**追问 1：为什么不直接对每个异常日志告警？**

**口播：** 单条日志容易受到重试、重复打印和采样影响，不能稳定代表用户影响；而且日志告警通常缺少请求总量，无法区分一次偶发错误和持续错误率。更合理的做法是用 Counter 和 Histogram 计算比例、分位数和预算消耗，日志作为根因上下文。比如 ES 偶发超时可以在日志里记录具体错误，但只有当依赖错误率或 Chat 失败率持续越过阈值时才升级告警。这样告警更少、更接近 SLO，工程师排查时又不会失去细节。

**追问 2：观测系统本身故障怎么办？**

**口播：** 观测是重要能力，但不能成为主请求的单点阻断。Exporter 使用批量队列和有限超时，发送失败只限频记录并增加丢弃计数，主请求继续按业务错误策略返回；ready 可以把观测标成 degraded，但不把它和 Redis、MySQL、ES 这类核心依赖混为一谈。应用退出时给 provider 一个明确的 flush deadline，超过 deadline 就放弃，避免优雅关闭被观测阻塞。恢复后要验证队列、丢弃和 trace 到达情况，确认降级是可见、可恢复的。

#### 主问 9：一次依赖故障你会怎样排查？

**口播：** 假设用户反馈 Chat 变慢，我先在 Prometheus 看请求量、5xx、p95 和依赖延迟，确认问题是否集中在某个 route 或 dependency；如果 ES 错误率升高，再用告警时间窗和服务标签到 Jaeger 查一条代表性 Trace，看 `retriever.elasticsearch` 的 DNS、连接、服务端响应还是查询本身耗时；随后用 trace_id 查结构化日志，确认错误码、超时和是否发生降级。修复可能是恢复 ES、调整超时、限流或回滚配置。最后我会重跑同一 curl、确认 ready 恢复、指标回落，并把故障记录、测试和 Runbook 一起补回仓库。

**追问 1：为什么 ready 失败和业务请求失败要分开？**

**口播：** ready 是给流量管理和部署系统看的，它回答“实例是否适合继续接收请求”；业务请求则要返回稳定错误码和 trace_id，帮助客户端或上层重试策略判断。比如 ES 停止时，ready 应该返回 503 并显示对应依赖失败，Chat 可能返回依赖不可用或有限降级结果，两者不能只返回一个模糊的 500。live 仍然只表示进程活着，避免依赖故障导致编排器不断重启本来还能提供部分能力的实例。这个分离也便于故障演练验证。

**追问 2：你如何避免重试把故障放大？**

**口播：** 重试必须由调用层根据错误可恢复性、幂等性、剩余 deadline 和预算共同决定，而不能对所有错误无条件重试。Agent 工具和依赖调用要限制次数并采用有上限的退避，context 取消或 deadline 到期时立即停止；写 MySQL 这类有副作用的操作要先确认幂等键或不重试。指标里分别记录原始失败、重试次数和最终结果，Trace 里记录每次 attempt，才能看出是依赖真的慢还是客户端重试制造了更多流量。

### D. Agent、RAG、SSE 与一致性

#### 主问 10：Agent 如何避免无限循环和越权调用？

**口播：** 我把模型能力放在服务端策略约束里。Runner 每轮先检查 context 和 deadline，再调用模型；如果模型产生工具调用，就校验名称、白名单、参数 JSON 和参数大小，并检查总工具调用预算。策略还限制最大 step，检测相同工具和规范化参数的连续失败，达到阈值就停止；工具失败根据 fallback 配置返回稳定错误或有限答案，并保留审计。用户请求不能直接修改这些上限，Eino 只负责供应商适配和 ReAct 能力，项目自己的 Runner 和 Registry 才是安全边界。

**追问 1：为什么还要保留手写 Loop？**

**口播：** 手写 Loop 是行为基准，不是为了重复造模型框架。它把最大轮数、工具预算、重复失败、取消和 fallback 这些项目真正关心的规则变成可控的 fake model 测试；Eino 集成则验证供应商消息、tool observation 和 callback 审计能否接上。两者对照可以发现框架默认行为和项目策略之间的差异，未来替换供应商或升级 Eino 时也有回归基线。这样我面试时能解释哪些能力由框架提供，哪些能力必须由项目自己保证。

**追问 2：工具审计记录什么，为什么不记录全部参数？**

**口播：** 审计至少记录工具名、step、结果、耗时、经过大小限制的参数摘要和安全错误摘要，用于回答“模型当时做了什么、哪一步失败”。完整参数可能包含用户输入、密钥或高维内容，所以默认截断、脱敏或只保留哈希，原始供应商错误写日志也要净化。审计不能阻塞最终回答主路径，后续可以异步落事件表，但 trace 中仍要保留关键的 tool span。这样既保留复盘证据，又控制隐私和存储风险。

#### 主问 11：SSE 客户端断开时，系统如何收尾？

**口播：** SSE 入口使用请求 context 启动流，Service 把上游模型增量归一成事件流，Handler 负责编码、写帧和 flush。正常 EOF 时才聚合完整 assistant 内容并保存 Redis/MySQL，然后发送 done；如果客户端断开，request context 取消，`Recv` 返回取消语义，Handler 停止写入，stream Close 保证幂等，上游模型和工具也必须收到取消，不保存半截 assistant。上游错误则发送稳定错误事件并关闭。这个顺序避免数据库留下半条事实，也避免 goroutine、连接和工具调用继续泄漏。

**追问 1：为什么不每个 token 都写 MySQL？**

**口播：** 每个 token 写一次会把一次回答变成大量数据库事务，放大锁、网络和存储压力，还会留下大量半成品记录。流式 token 属于传输状态，只有正常结束并聚合成完整 assistant 消息后才是长期事实；客户端中途断开时不保存半截内容。若产品未来需要断点续传，可以单独设计临时流状态、版本和过期策略，而不是把传输增量直接当历史消息。当前实现选择完整回答一次写入，边界清晰、便于重试和审计。

**追问 2：如何证明取消真的传到了下游？**

**口播：** 我会用可控的 fake token stream 或延迟模型，在收到请求后主动取消客户端 context，再断言 service 的 `Recv` 返回 context canceled、底层 stream 的 Close 只调用一次、后续 token 不再被消费，而且 MySQL/Redis 没有 assistant 记录。真实模型接入后再用相同场景做集成验证，并在 Trace 中观察 `chat.stream`、LLM 和工具 Span 的结束状态。指标可以补充取消计数和取消后的下游调用计数，避免只看 HTTP 连接断开就误以为资源已经释放。

#### 主问 12：为什么 Redis 和 MySQL 要同时存在？

**口播：** 两者优化目标不同。Redis 保存每个 session 的近期窗口和 TTL，适合高频读取和控制上下文长度；MySQL 保存完整会话历史，是事务和审计事实。读取时优先 Redis，未命中回源 MySQL 并尝试回填；写入时先保证 MySQL 成功，再更新 Redis，Redis 失败不能撤销已经提交的事实，只记录告警并保留补偿空间。这样缓存失效不会等价于数据丢失，数据库抖动也不会被伪装成空上下文。真正的并发顺序和 Compose 集成还需要继续补验收。

**追问 1：如果 Redis 读取失败，为什么不能直接返回空上下文？**

**口播：** 直接当成空上下文会让模型误以为这是新会话，可能重复提问、产生错误结论，还掩盖了依赖故障。当前默认策略是把 Redis 读取错误变成明确失败；如果未来启用降级，也必须由配置显式允许，并记录 degraded 原因和 trace_id，必要时只读 MySQL。降级的边界要在测试里写清楚：哪些接口可以空上下文，哪些接口必须阻断，恢复后如何回填。任何“为了可用而吞掉错误”的做法都要有可观测证据和恢复策略。

**追问 2：多请求同时写同一 session 怎么保证顺序？**

**口播：** 当前仓库已经有批量写入和窗口裁剪，但严格的同 session 并发顺序仍列为待补项，我不会声称它已经解决。生产方案需要先定义顺序来源，例如服务端生成单调版本、数据库唯一约束加事务，或者按 session 做分布式串行化；Redis 列表更新和 MySQL 事实提交必须使用同一个可验证的版本语义。验证时要并发发送带序号的消息，检查 MySQL、Redis 窗口和历史读取顺序一致，再在 Redis 或 MySQL 故障中重复。这个问题属于一致性设计，不应该靠“通常按到达顺序”解释。

### E. 企业化交付与演进

#### 主问 13：企业级优化你会优先做哪些？

**口播：** 我会按风险和证据排序。第一优先级是 Stage 06 P0，把 HTTP、Chat、Agent、工具和依赖串成 Metrics、Trace、Logs 闭环；第二是完成 Compose smoke、真实模型取消、Redis/MySQL 并发和 ES 故障演练；第三是补 SLO、告警、Dashboard、采样、队列、脱敏和 Trace 数据保留；第四才是基于指标决定是否拆知识检索服务。工程上还要强化配置校验、API 鉴权、限流、幂等、优雅关闭和 CI 质量门禁。每项优化都要绑定一个可执行验收命令或测试证据，避免把“加组件”当成企业级。

**追问 1：为什么可观测性要排在服务拆分前？**

**口播：** 没有观测，拆分后只会把一个进程内的问题变成多个网络 hop 的问题。我们需要先知道请求量、p95、依赖错误、Agent 工具失败和 token 成本，才能判断哪个边界真的需要独立扩缩容或隔离；还要能用 Trace 证明跨边界的 timeout、重试和上下文透传正确。先在单体中完成 root span、子 span、低基数指标和故障演练，可以把这些契约固定下来，后续迁移到 gRPC 或消息队列时复用，降低重构风险。

**追问 2：还有哪些安全和成本边界？**

**口播：** 安全上要做 API Key 或服务间身份认证、输入大小限制、工具白名单、日志和 Trace 脱敏、DSN 与模型密钥不落盘，并对观测 UI 做访问控制。成本上要限制 Agent step、工具次数、模型 token、RAG top-k 和 Trace 采样率，Exporter 使用有界队列和超时，避免观测反过来耗尽资源。Prometheus 标签保持低基数，Jaeger 只采样必要请求，详细 prompt 不进长期存储。每个边界都应该对应配置、指标和故障测试，而不是只写安全说明。

#### 主问 14：你如何设计这条链路的测试与验收？

**口播：** 我会分四层验收。单元测试覆盖纯函数、RRF、策略停止、错误映射和配置校验；集成测试覆盖 Redis、MySQL、ES 的真实协议和索引；HTTP 测试覆盖同步、SSE、取消、ready 和 `/metrics`；端到端 smoke 则启动 Compose，执行健康检查、知识导入、检索、Chat、指标抓取和 Jaeger Trace 查询。故障演练分别停止 Redis、MySQL、ES、OTLP exporter，并记录返回码、指标变化、Trace 错误和恢复时间。当前 `go test ./...` 已通过，但 Prometheus/Jaeger 的端到端验收要等 P0 接线后执行。

**追问 1：如果本地没有真实模型，怎么验证 Agent？**

**口播：** 项目已经保留 mock 模型和手写 Loop 基准，让本地可以验证工具白名单、预算、重复失败、fallback、记忆读写和多代理路径；真实 Eino tool-calling 集成再用 OpenAI-compatible 配置补一次。关键是把模型供应商替换在 `internal/ai` 适配层，业务测试不依赖外部 API。对于 Trace，我可以先用 mock Chat 产生确定的 `http.request`、Agent 和工具 Span，再用真实模型验证 token、上游错误和长尾。这样单元、集成和供应商验证各自回答不同问题，不会因为没有 API Key 就失去工程闭环。

**追问 2：哪些结果你现在不能声称？**

**口播：** 我不能声称 Prometheus 已经持续抓到 Pilot 指标，不能声称 Jaeger 已经保存并展示生产 Trace，也不能声称 Stage 07 的全部 Compose 冒烟、Stage 02 的并发顺序和真实模型取消都已经完成。对于 RAG，我可以声称 BM25、向量 kNN、RRF 和基础 ES 集成已经有代码和测试，但不能编造召回率提升百分比。面试中我会把“已实现”“已有测试”“待测”“设计目标”分开说，并给出下一步命令和通过标准，这比报一个没有证据的性能数字更可信。

#### 主问 15：如果让你把观测能力落地，你会怎样分批提交？

**口播：** 我会拆成可回滚的小批次。第一批只增加 Prometheus registry、低基数 HTTP metrics、`/metrics` 和 scrape 验收；第二批增加 OTel SDK、OTLP gRPC exporter、resource 和 shutdown flush；第三批接 HTTP root span、Trace-ID 响应头和 slog 关联；第四批接 Chat/SSE、Agent、Tool、Redis、MySQL、ES 的业务子 Span；第五批补告警规则、故障演练、Dashboard 和 CI smoke。每批都先补测试和 curl，再进入下一批，避免一次提交同时改变请求、指标、追踪和部署。完成后才能用一条真实故障演练讲清“告警 -> Trace -> 日志 -> 修复 -> 回归”。

**追问 1：为什么不直接引入一个大而全的可观测平台？**

**口播：** 目前项目处于单体学习和边界收敛阶段，先用 Prometheus、OpenTelemetry 和 Jaeger 建立标准接口，能保留供应商替换空间，也能让每个数据源的职责清楚。大平台可能帮我们快速看到图表，但如果没有低基数指标、Trace parent、日志脱敏、失败降级和验收测试，问题仍然无法解释。先把数据契约和闭环做对，再接 Grafana、Collector 或托管平台，迁移成本更低。企业级的核心是可验证、可恢复和可演进，不是平台名称越多越好。

**追问 2：你如何把这段经历讲成一个闭环结果？**

**口播：** 我会用一个可复现的 ES 故障场景来讲。先用 Prometheus 发现 Chat 的依赖错误率和 p95 上升，告警里带 route 与 dependency；再拿请求返回的 trace_id 到 Jaeger，看到 `retriever.elasticsearch` Span 的超时和父子耗时；通过结构化日志确认错误码和是否走了允许的降级；修复 ES 或调整经过验证的超时策略后，重新执行同一条 Chat curl，检查 ready 从 503 恢复 200、错误计数停止增加、Trace 变为成功。最后把故障步骤、回归测试和 Runbook 提交到仓库。当前这条口播描述的是 Stage 06 的验收目标，完成接线后再把实际输出补入。

## 4. 源码证据索引

| 主题 | 关键路径与内部符号 | 对应正文位置 |
| --- | --- | --- |
| 启动与依赖 | `cmd/pilot/main.go`、`cmd/pilot/wiring.go`、`deps.CheckAll` | 主问 2、3、9、14 |
| HTTP 与健康 | `internal/handler/*.go`、`HealthHandler.Live/Ready` | 主问 1、9、14 |
| 双层记忆 | `internal/memory/service.go`、`RedisStore`、`MySQLStore` | 主问 12 |
| SSE | `internal/chat/stream.go`、`internal/handler/chat_stream.go` | 主问 11 |
| RAG | `internal/ai/retriever/rrf.go`、`ESRetriever`、`internal/ai/prompt/` | 主问 1、10 |
| Agent 策略 | `internal/agent/loop.go`、`policy_runner.go`、`AgentPolicy` | 主问 10 |
| 工具与审计 | `internal/tools/registry.go`、`internal/tools/health_check.go`、`internal/ai/eino/audit.go` | 主问 10 |
| 多代理 | `internal/multiagent/orchestrator.go`、`taskstate.go` | 主问 1、2 |
| 观测部署现状 | `manifest/docker-compose.yml`、`manifest/prometheus/prometheus.yml` | 主问 4、5 |
| 观测施工规格 | `docs/build/implementation/spec/11-observability-closed-loop.md`、`docs/build/stages/stage-06-observability.md` | 主问 4-9、13-15 |
| 测试与阶段证据 | `docs/build/testing/README.md`、`stages/CHANGELOG.md`、`stages/TODO.md` | 主问 2、14 |

## 5. 交给 /asu 的项目事实摘要

- 项目名称：Pilot；方向：Go 后端 / AI 应用 / 运维排障。
- 个人职责边界：分层请求链路、双层记忆、SSE 生命周期、RAG 基础、Agent 策略与工具审计、施工规格和测试设计。
- 关键技术动作：Redis + MySQL 记忆职责分离；ES BM25 + kNN + RRF；Agent 工具白名单、预算、重复失败停止和 fallback；SSE 取消传播；Prometheus/OTel/Jaeger 闭环设计。
- 可核验证据：`go test ./...` 通过；根模块有对应包和测试；Compose 已声明 Redis、MySQL、ES、Prometheus、Jaeger；`11-observability-closed-loop.md` 给出接线和验收命令。
- 候选表述：分层请求编排、双层记忆一致性、可控 Agent 执行、混合检索证据链、可观测性工程化。
- 待补事实：Stage 06 应用接线、真实 Trace 截图或导出、Compose 全链路验收、Redis/MySQL 并发顺序、真实模型取消和可量化 SLO。

## 6. 交给 /interview 的高风险 Claim 清单

- **Ownership Claim：** “负责/主导”只用于本人确实完成的分层、策略、测试和规格；不把未来 P0 接线说成已上线。
- **Metric Claim：** 当前没有可确认的 QPS、延迟、召回率提升或成本下降数字；应使用待测指标和验收方法。
- **Architecture Claim：** 可以讲清 Redis/MySQL、RRF、Agent policy、SSE cancel 和目标观测架构；跨服务 Trace 透传仍属于 Stage 09 练习。
- **Result Claim：** 可以说 `go test ./...` 当前通过、核心业务骨架已接通、Compose 观测容器已声明；不能说 Prometheus 已持续采集或 Jaeger 已有生产 Trace。
