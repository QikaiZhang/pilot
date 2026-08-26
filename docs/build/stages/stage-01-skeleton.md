# Stage 01：Go 工程骨架与基础设施

## 目标

从文档中的最小启动模型开始建立可维护的 Go 服务：配置、日志、路由、依赖检查、优雅关闭和测试入口。

## 实现顺序

1. `envloader` 读取 `.env`，但不覆盖已有环境变量。
2. `pkg/configutil` 将环境变量映射到配置结构。
3. `/api/v1/health/live`、`/api/v1/health/ready` 路由和统一错误响应。
4. 启动时检查 Redis、MySQL、ES 的连通性。
5. `context.Context` 贯穿初始化和关闭流程。

## 手写/AI 边界

- S：优雅关闭、依赖检查失败聚合、错误到 HTTP 状态的映射。
- A：配置结构、路由分组、Repository 接口。
- B：Makefile、Compose、日志格式和重复初始化代码。

## 验收

- 依赖不可用时服务能给出明确错误并退出；可选观测能力只在允许降级时继续启动。
- `go test ./...` 可运行；启动顺序能口述。
