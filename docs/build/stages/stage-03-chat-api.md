# Stage 03：Eino 聊天闭环、SSE 与 Agent Loop

## API 范围

- `POST /api/v1/chat`：同步对话。
- `POST /api/v1/chat/stream`：SSE 流式对话。
- `GET /api/v1/chat/history`：历史查询。
- `DELETE /api/v1/chat/history`：历史清理。

## 推进策略

本阶段分成两条连续路线：先使用 Eino 打通可运行闭环，再把关键边界抽出来做手写增强训练。生产路径保留 Eino 实现，手写版本用于理解、测试和面试复盘，不重复实现框架内部机制。

### 第一段：Eino 基线闭环

1. 定义项目自己的 `ChatModel`、请求、响应和流式事件契约。
2. 使用 Mock ChatModel 跑通同步聊天和 `memory.Service`。
3. 接入 Eino ChatModel 的真实 `Generate` 调用。
4. 接入 Eino `Stream` 和 SSE 输出适配器。
5. 使用一个确定性工具接入 Eino ReAct，跑通一次工具调用。

### 第二段：增强训练

1. 手写 SSE 读取、Flush、EOF、错误和客户端取消模板。
2. 手写最小 Agent Loop，明确最大轮数、工具失败、超时和停止条件。
3. 将手写版本与 Eino 基线做行为对照测试，记录框架解决的问题和项目保留的策略。

## 手写/AI 边界

- 基线阶段：Eino 适配、路由注册和重复序列化属于 A/B，先保证可运行。
- 增强阶段 S：SSE 事件循环、客户端断开后的取消传播、错误事件格式、Agent 停止条件。
- A：请求 DTO、响应 VO、参数校验和 ChatService 组装。
- B：重复路由注册、基础序列化和配置装配。

同步和 SSE 都要先做相同的输入规范化：`user_id`、`session_id`、`query` 分别限制长度并拒绝空问题。SSE 正常结束后再落 assistant 消息，客户端提前断开则取消下游 context；不要把每个 token 都写进 MySQL。

## 当前落地范围与风险

当前已完成同步 Chat 的运行时闭环，以及 `chat.Service` 内部的 Stream 基线：HTTP 请求 -> `chat.Service` -> `memory.Service` -> `ai.ChatModel` -> JSON 或事件流。模型实现由组合根根据 `LLM_MODE` 选择 Mock 或 Eino OpenAI-compatible 适配器；Embedding 暂不接入，保留 `EMBEDDING_MODE=mock`，等 Stage 04 的 ES/RAG 检索链路再处理。

主要风险是供应商的 OpenAI-compatible 地址和模型名可能不同。`LLM_BASE_URL` 应填写 SDK 会继续拼接 `/chat/completions` 的基础地址；真实 API 调用先通过最小接口验证，再进入 SSE。同步链路验证通过后，才开始手写取消传播、流关闭和 Agent Loop 等 S 级模块。

## 本轮完成与下一轮 Review

本轮完成了 `chat.Service.Stream`、`serviceStream.Recv/Close`、assistant 的 EOF 后持久化和服务层测试。`POST /api/v1/chat/stream` 已注册（`internal/controller/chat_stream.go`）并通过真实链路验证：SSE 输出 content + done 帧、`data: ...\n\n` 分隔、客户端提前断开不落 assistant。下一轮优先 review：上下文与历史消息如何进入模型、user/assistant 的保存时机、EOF 与 `io.EOF` 的区别、取消和 `Close` 的幂等性，以及错误事件是否泄露供应商信息。

### S 级待手写清单（已标记，后续独立练习）

以下段落已在源码中用 `BEGIN/END S_LEVEL_REFERENCE` 标注，学习者在验收前应从零独立重写，禁止直接复用：

- SSE 事件循环（`controller/chat_stream.go` 的 `for { Recv -> writeSSEEvent -> Flush }`）：EOF/错误/取消三种终止的分支判断。
- 客户端断开后的取消传播：`r.Context()` 如何取消下游 `TokenStream`，以及 context.Canceled 与 io.EOF 的区别。
- 错误事件格式：供应商错误如何映射为不泄露原始错误的可读事件。
- Agent 停止条件：最大轮数、工具失败、超时（Stage 05 工具链，先记录）。

## 验收

- Eino 基线能跑通：同步 Chat -> 记忆读取 -> 模型 -> 消息保存 -> JSON 返回。
- Eino 流式路径能跑通：模型 Stream -> SSE token -> 完整 assistant 消息保存。
- 至少一个工具能完成一次 ReAct 调用并返回结果。
- 手写增强版本覆盖正常、超时、取消、工具失败和最大轮数。
- 同步和 SSE 共享同一业务编排，只有输出适配器不同。
