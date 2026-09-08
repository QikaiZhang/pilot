# Agent 与工具调用规格

本规格是 Stage 05 的施工依据，描述 Agent Runner、模型工具调用协议、工具注册和第一条验收链路。Eino 只能位于适配层，不能成为 `chat` 或业务层的公共契约。

## 分层边界

- `internal/ai`：供应商无关的模型协议，包括工具定义、模型发出的工具调用和工具结果。
- `internal/agent`：Agent 编排、策略限制、停止条件和工具审计记录。
- `internal/tools`：具体工具和 Registry。工具不决定下一步调用谁。
- `internal/ai/eino`：把项目契约转换为 Eino 类型的适配器。

## Runner 闭环

```text
AgentRequest
  -> 读取历史并请求模型
  -> 模型返回回答或 ToolCall
  -> Registry 按名称和白名单查找工具
  -> Execute(ctx, arguments)
  -> ToolResult 作为 RoleTool 消息交回模型
  -> 达到最终回答、MaxSteps 或失败回退
```

`AgentPolicy` 由 Runner 持有，不从普通用户请求读取。上层 `context.Context` 必须透传；Runner 可以基于策略派生超时子 context，但不能改用 `context.Background()`。

## 第一条运行时链路

`POST /api/v1/agent/chat` 是本阶段的同步验收入口：`AgentHandler` 解析请求，`chat.AgentService` 加载历史并保存用户消息，`agent.Runner` 调用 Eino ReAct，最后保存助手答案并返回 `AgentResponse`。普通 `/api/v1/chat` 和 SSE 路径暂不改为 Agent，便于对照测试。

当 `LLM_MODE=mock` 时，Agent 依赖不会被组装，该入口返回 `503 agent_unavailable`；设置真实的 OpenAI-compatible API 配置后才会执行 ReAct 和 `health_check` 工具。

## 项目契约

模型工具调用与审计记录必须分开：模型调用只表达意图（`ID/Name/Arguments`），`agent.ToolCall` 记录执行轮次、状态和经过净化的错误。一次模型响应允许包含多个工具调用，即使第一版按顺序执行。

工具至少提供名称、描述、JSON Schema 参数和执行函数。第一版工具顺序固定为 `health_check`、`knowledge_search`；健康检查返回所有依赖的 `name/status/error`，不得泄露密码、DSN 或内部堆栈。

## 失败与停止

- 空回答且无工具调用：结束并返回模型结果。
- `MaxSteps`、context 超时或取消：停止循环并写入 `limitations`。
- 工具不存在、不在白名单或执行失败：记录失败 ToolCall；按策略选择无工具回答或返回错误。
- 不允许无限重试；工具错误只向用户暴露稳定的摘要。

### 重试与多工具调用

`MaxSteps` 是整个 Agent Loop 的兜底上限，不是工具重试次数。对于明确可重试的瞬时错误，工具执行器可以在单次 ToolCall 内采用有限次数的指数退避；退避必须受剩余 context deadline 约束，不能为了重试突破 Runner 总超时。模型连续生成相同失败调用不等同于瞬时重试：Runner 应记录重复调用，并在达到重复失败阈值后停止或进入 fallback，不能只依赖 `MaxSteps`。

一次模型响应包含多个工具调用时，第一版按返回顺序顺序执行。只有在工具契约明确声明无依赖、可并行且结果不互相影响时，才允许并行；如果存在依赖关系，应按依赖顺序执行，前置工具失败则跳过依赖工具，并将 limitation 写入响应。Runner 不应通过工具名称猜测依赖关系。

## 策略函数要求

策略函数只做边界判断或 context 派生，不访问模型、数据库或工具实现。`Validate` 拒绝非正的 `MaxSteps/Timeout`、空白或重复白名单；`AllowsTool` 对空白名称一律拒绝；超时 context 必须从上层 context 派生并保留取消信号。

## 验收

1. 健康检查工具可在无 Redis/MySQL/ES 的单元测试中运行。
2. 模型请求能携带工具定义，工具结果能重新进入上下文。
3. 工具失败不会无限循环，审计记录包含名称、参数、step 和状态。
4. `health_check -> knowledge_search -> 最终回答` 能在集成测试中闭环。
