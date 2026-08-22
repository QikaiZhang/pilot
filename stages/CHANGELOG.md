# 训练变更记录

> 本文件记录我们的阶段实训变更（与 `docs/CHANGELOG.md` 课程资料分离）。

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
- `docs/` 保留项目开发规格与课程参考；本目录独立记录本次实训进展，避免把学习过程混入开发契约。

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
