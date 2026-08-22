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
- `docs/CHANGELOG.md`、`docs/THOUGHT.md` 还原为原始课程资料，本目录独立记录实训进展。

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
