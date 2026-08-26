# 训练变更记录

> 本文件记录我们的阶段实训变更（与 `docs/history/CHANGELOG.md` 课程资料分离）。

## Stage 01：工程骨架（进行中）

### 新增能力

- 新增 `pkg/envloader`：`.env` 读取，已有环境变量优先不覆盖。
- 新增 `pkg/configutil`：环境变量到 `Config` 结构的映射，默认值对齐 spec。
- 新增 `internal/deps`：启动期 Redis/MySQL/ES 连通性检查，失败聚合、错误净化。
- 新增 `internal/controller`：`/api/v1/health/live`、`/api/v1/health/ready` 与统一错误体。
- 新增 `cmd/pilot`：配置 → 依赖检查 → HTTP → 优雅关闭的组装入口。
- 新增 `manifest/docker-compose.yml`、`manifest/prometheus/prometheus.yml`、`.env.example`、`Makefile`、`.gitignore`。
- 新增 `stages/`：阶段实训沉淀目录（源码拆解模板、S 级手写指南、面试复习卡、本记录与思考记录）。

### 改动范围

- `cmd/main.go`（HelloWorld）迁移为 `cmd/pilot/main.go` 正式入口。
- `go.mod` 增加 `go-redis/v9`、`go-sql-driver/mysql` 依赖。
- `docs/` 保留项目开发规格与课程参考（施工归 `docs/build/`，复盘/面试归 `docs/review/`，变更记录归 `docs/history/`）；本目录独立记录本次实训进展，避免把学习过程混入开发契约。

### 数据流变化

- 入口：`go run ./cmd/pilot` → `envloader.Load(".env")` → `configutil.Load()`。
- 处理：`deps.CheckAll` 启动期聚合检查三依赖（10s 超时），失败即退出。
- 存储/外部调用：Redis/MySQL/ES 仅连通性探测，无业务读写。
- 返回/副作用：HTTP `:8080`；`/live` 200；`/ready` 200/503。

### 当前局限

- `configutil` 对非法配置值静默回退默认值，缺 `Validate()`。
- `ALLOW_DEGRADED` 已定义未消费（观测依赖降级留给 Stage 06）。
- S 级核心逻辑仍为 AI 参考实现，待学习者按路线 B 删除重写。

### 后续优化

- 配置强校验；`/ready` 并发化或缓存上次结果；依赖正式 client 化（Stage 02+）。

### 验收结果

- 代码运行：`go build ./...`、`go vet ./...` 通过；无依赖启动时聚合报错并退出。
- 测试结果：envloader / configutil / controller 单测通过，`go test ./...` 绿。
- S级手写确认：待学习者完成（路线 B，见 `stage-01/s-level-handwriting-guide.md`）。
- Review 结论：设计复盘与字段建模评分已写入 `THOUGHT.md`；S 级手写后做 Diff Review 与口述验收。

## Stage 02：记忆与持久化（核心实现完成）

### 新增能力

- 新增 `internal/memory` 的消息模型、存储端口、MySQL 历史存储和 Redis 短期记忆。
- 新增 `memory.Service`，协调 Redis 缓存命中/回源和 MySQL 优先写入。
- 新增 sqlc schema、queries 和生成代码，表结构与消息字段保持明确映射。

### 改动范围

- `internal/memory/mysql.go`：批量消息事务写入、历史查询、删除和 `CreatedAt` 零值处理。
- `internal/memory/redis.go`：会话 key 编码、List 窗口、TTL、批次校验和清理。
- `internal/memory/service.go`：`LoadRecent`、`SaveMessages` 和缓存失败告警。
- `internal/memory/*_test.go`：模型、适配器边界和 Service fake store 测试。

### 数据流变化

- 读取：Redis 命中直接返回；未命中回源 MySQL，成功后尝试回填 Redis。
- 写入：校验消息批次 → MySQL 持久化 → Redis 更新近期窗口。
- 失败：MySQL 失败阻断写入；Redis 失败不撤销 MySQL，保留告警和后续补偿空间。

### 当前局限

- 尚未接入聊天 Controller，Service 还没有真实 HTTP 调用链。
- 尚未完成 Compose 环境下的 MySQL/Redis 集成测试。
- 同一会话并发写入的严格顺序和补偿机制留到后续阶段。

### 验收结果

- 代码运行：`go test ./...` 通过。
- 测试结果：Service fake store 覆盖命中、回源、错误和缓存失败策略。
- S 级手写确认：Service 核心路径已完成 Review，需在阶段口述验收中确认理解。
- Review 结论：核心持久化链路可进入 Stage 03；集成验证作为后续补强项。

## Stage 03：聊天闭环、SSE 与本地基础设施(基础完成)

### 新增能力

- `POST /api/v1/chat/stream`：SSE 流式对话，由 `internal/controller` 的 `Chat.Stream` 适配。业务编排留在 `chat.Service.Stream`——先 `LoadRecent`，再保存 user 消息，最后启动模型流。
- 三层流模型落地：Eino/SDK 适配成 `ai.TokenStream`（裸增量）→ `serviceStream` 归一成 `chat.EventStream`（content/done/error + 生命周期）→ Controller 编码成 SSE 帧。每层只做一次转换，互不泄漏。
- `serviceStream.Recv/finish/Close`：累积增量；上游正常 EOF 时才保存完整 assistant；空增量跳过；`Close` 幂等；上游错误映射为 `UPSTREAM_ERROR`，空响应映射为 `EMPTY_RESPONSE`。
- LLM options 配置装配：`LLM_TEMPERATURE`/`LLM_MAX_TOKENS`/`LLM_TIMEOUT` 从 env 读入 `LLMConfig`，经 `buildChatModel` 传入 Eino 适配器（仅 `LLM_MODE=eino/openai` 消费，mock 不生效）。
- Makefile 本地基础设施：`infra-up`（复用本地 `mysql:8.0` 与 `elasticsearch:8.18.8` 镜像，ES 自动等待至非 red）、`infra-down`、`infra-logs`、`stream`。Redis 复用已运行的 redis-stack-server，不覆盖 compose。

### 改动范围

- `internal/controller/chat_stream.go`：SSE Handler 主体，含 `defer stream.Close()`、`writeSSEEvent` + `Flush`。
- `internal/controller/chat_stream_test.go`：用 `flushRecorder` 模拟 `http.Flusher`，断言 content/done 帧、`\n\n` 分隔、EOF 结束、底层流关闭一次。
- `pkg/configutil`：`LLMConfig` 增加 `Temperature/MaxTokens/Timeout`，`Load()` 读对应 env，新增 `getFloat` helper。
- `cmd/pilot/wiring.go`：`buildChatModel` 传入 LLM options。
- `cmd/pilot/main.go`：注册 `POST /api/v1/chat/stream`。
- `Makefile`：新增 `infra-up/infra-down/infra-logs/stream` 目标。
- `.env`：追加 `LLM_TEMPERATURE`/`LLM_MAX_TOKENS`/`LLM_TIMEOUT` 注释行。

### 数据流变化

- 同步：`POST /api/v1/chat` → `chat.Service.Generate` → `LoadRecent` → 存 user → `ChatModel.Generate` → 存 assistant → JSON。
- 流式：`POST /api/v1/chat/stream` → `Service.Stream` → `LoadRecent` → 存 user → `ChatModel.Stream` → 逐帧 `data: {...}\n\n` + `Flush` → EOF 存 assistant 并返回 `done`。
- 取消：客户端断开 → `r.Context()` 取消 → `Recv` 返回 `context.Canceled` → Controller 静默断开，不保存 assistant。

### 当前局限

- `stage-03-chat-api.md` 第 45 行仍写着「SSE Controller 尚未注册」，与本轮实际不符，待更新。
- LLM options 仅在 eino/openai 模式下消费；当前 `.env` 为 `LLM_MODE=mock`，故未真正压测到厂商参数。
- `ALLOW_DEGRADED` 仍为未消费字段（依赖检查为硬性），本次未改动 `main.go` 启动逻辑。
- 客户端取消（u9 会话）实测未落库、符合预期，但依赖 mock 流首帧延迟，真实模型的取消边界待接入后复核。

### 验收结果

- 代码运行：`go test ./...`、`go vet ./...` 通过，`gofmt` 干净。
- 基础设施：`make infra-up` 成功，ES 自动等待至 green；`/health/ready` 三依赖全 ok。
- 接口实测：`/chat` 返回 JSON 含 usage；`/chat/stream` 输出 `content` + `done` 两帧，`\n\n` 分隔正确。
- 持久化实测：MySQL `conversation_history` 落库 user+assistant，顺序正确，UTF-8 存储正常（`E5B7B2`=已）。
- 取消实测：客户端提前断开后目标会话无记录落库。
- 下一步：更新 `stage-03-chat-api.md`；接入真实 eino/LLM 后复核 SSE 真实厂商流；再进入 S 级手写 SSE 事件循环/取消传播。

## Stage 03：S 级标注与流式心智模型（收尾）

### 新增能力

- 为 SSE 事件循环打上 S 级标注：`internal/controller/chat_stream.go` 的 `for { Recv -> writeSSEEvent -> Flush }` 用 `BEGIN/END S_LEVEL_REFERENCE` 标注，标记为待学习者独立重写。
- 修正 `docs/build/stages/stage-03-chat-api.md` 的文档失真（「SSE Controller 尚未注册」→ 已注册并实测通过），并新增「S 级待手写清单」小节的显式清单。
- 新增 `docs/build/architecture/streaming-layers.md`：三次流、两次转换、每层唯一转换点的代码级心智模型。
- `docs/build/architecture/README.md` 索引加入 streaming-layers。

### 改动范围

- 仅在代码中加注释（`chat_stream.go` 的 S 级标注块）与更新文档，**未改变任何运行逻辑**。
- `stage-03-chat-api.md`：更新现状 + 待手写清单。
- `stages/CHANGELOG.md`：本记录。
- `docs/build/architecture/streaming-layers.md`：新增。

### 数据流变化

- 无。纯注释与文档沉淀。

### 当前局限

- S 级 SSE 事件循环、取消传播、错误事件格式仍未手写，仅完成标注与文档，待学习者按清单独立练习。
- Agent 停止条件（最大轮数/工具失败/超时）属 Stage 05，已记录但未实现。
- 长/输入长度限制（`user_id`/`session_id`/`query` 限长）尚未实现，当前 `normalizeTurn` 仅做 TrimSpace。

### 验收结果

- 代码运行：`go test ./...` 通过；`chat_stream.go` 加注后仍通过。
- 文档：`stage-03-chat-api.md`、`streaming-layers.md`、README 索引齐全。
- 下一步：学习者按 S 级清单手写后 Diff Review；再进 Stage 04（ES/RAG）。

## Stage 03：文档目录重组（施工 / 复盘 / 历史分离）

### 新增能力

- 将 `docs/` 顶层重组为三块，施工与复盘彻底分离，按用途检索更方便：
  - `docs/build/`：施工规格（原 `development/architecture/api/implementation/testing/glossary/stages`）。
  - `docs/review/`：复盘与面试（原 `interview/` + `THOUGHT.md`）。
  - `docs/history/`：训练变更记录（原 `CHANGELOG.md`）。
- 新增三个入口索引：`docs/build/README.md`、`docs/review/README.md`、`docs/history/README.md`。

### 改动范围

- 用 `git mv` 保留历史：施工类目录迁入 `docs/build/`；`interview/`、`THOUGHT.md` 迁入 `docs/review/`；`CHANGELOG.md` 迁入 `docs/history/`。
- 修正所有交叉引用：`docs/README.md`、根 `README.md`、`docs/build/development/*`、`docs/build/stages/stage-08-interview.md`、`stages/{CHANGELOG,README,THOUGHT}*.md`、`stages/stage-01/*`。
- 内部相对链接逐一校验通过（脚本扫描全部 `.md`，无断链）。

### 数据流变化

- 无。纯文档目录与引用整理。

### 当前局限

- `docs/review/` 与 `docs/review/interview/` 内容未合并，面试沉淀保留在 `interview/` 子目录，属既有结构。

### 验收结果

- 全量 `go test ./...` 通过（7 ok）；`git diff --check` 干净；全部 `.md` 相对链接校验通过。
- 结构：`docs/` 顶层仅 `build/`、`review/`、`history/` 与 `README.md`，符合「施工 / 复习 / 历史」三分。

## 后续

- 学习者按 S 级清单手写后 Diff Review；再进 Stage 04（ES/RAG）。
