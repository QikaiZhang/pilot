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
