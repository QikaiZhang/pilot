# Pilot 总体架构

## 1. 系统定位

Pilot 是面向运维场景的 AI 助手，整合聊天、告警/指标/日志查询和知识库检索。课件当前采用 GoFrame HTTP Server 的单体形态，未来可在边界稳定后演练服务拆分。

## 2. 分层

```text
浏览器 / SPA
    -> HTTP Router / Middleware
    -> Controller（同步 / SSE）
    -> Agent Orchestrator（Eino / ReAct）
    -> Memory、Retriever、Tools
    -> Redis / MySQL / Elasticsearch / 外部模型
    -> Metrics / Trace / Logs
```

代码职责映射见 [开发文档](../development/README.md)。启动顺序是：环境加载 -> Redis/ES/MySQL 初始化 -> Controller 创建 -> 可观测性初始化 -> 静态资源和 `/api/v1` 路由挂载 -> 启动服务。

## 3. 关键取舍

- 先单体后微服务：降低学习和部署成本，保留 Controller、Agent、存储适配器等未来拆分边界。
- 强依赖启动失败：保证演示时完整链路可用，代价是暂时缺少降级能力。
- 同步与 SSE 共用业务编排：避免两份核心逻辑长期漂移。

同步/SSE 的详细时序见 [chat-data-flow.md](chat-data-flow.md)，Agent 图见 [agent-graph.md](agent-graph.md)。

## 4. 依赖可用性

启动阶段检查 Redis、MySQL、ES、模型和 Embedding 等核心依赖；可观测性上报等非核心能力允许在配置打开时降级，但降级必须有日志和健康状态。运行时依赖失败不能被吞掉，要转成稳定的业务错误或明确的 best-effort 结果。
