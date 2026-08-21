# Stage 03：聊天 API、SSE 与错误处理

## API 范围

- `POST /api/v1/chat/`：同步对话。
- `POST /api/v1/chat/stream`：SSE 流式对话。
- `GET /api/v1/chat/history`：历史查询。
- `DELETE /api/v1/chat/history`：历史清理。

## 手写重点

- S：SSE 事件循环、客户端断开后的取消传播、错误事件格式。
- A：请求 DTO、响应 VO、参数校验和 Controller 骨架。
- B：重复路由注册、Swagger/Apifox 样板和基础序列化。

同步和 SSE 都要先做相同的输入规范化：`user_id`、`session_id`、`query` 分别限制长度并拒绝空问题。SSE 正常结束后再落 assistant 消息，客户端提前断开则取消下游 context；不要把每个 token 都写进 MySQL。

## 验收

- 同一业务逻辑能被同步和流式接口复用。
- 能用 curl 或测试客户端验证正常、超时、取消和参数错误。
- SSE 事件至少覆盖文本片段、`[DONE]` 和结构化错误三种结果。
