# 流式对话与 SSE 规格

## 目标

流式接口只改变输出方式，不改变聊天业务编排。同步和流式都必须读取同一会话的近期上下文，并保存 user 消息；assistant 消息只有在模型正常结束后才允许持久化。

## 分层职责

`internal/chat` 是业务层。`Service.Stream` 负责规范化 `TurnRequest`、调用 `LoadRecent`、保存 user 消息、启动 `ai.ChatModel.Stream`，并返回 `EventStream`。`serviceStream` 累积模型增量，在上游 EOF 时保存完整 assistant。

`internal/handler` 是协议层。SSE Handler（`POST /api/v1/chat/stream`，`ChatHandler.Stream`）负责解析 JSON、设置响应头、循环读取 `EventStream`、写入 `data: ...\n\n` 并调用 `http.Flusher.Flush`。它不直接访问 Redis、MySQL 或供应商 SDK。

三层转换点：Eino/SDK 适配成 `ai.TokenStream`（裸增量），`Service.Stream` 把 `TokenStream` 归一成 `chat.EventStream`（content/done/error + 生命周期），Handler 把 `EventStream` 编码成 SSE 帧。每一层都只做一次转换，互不泄漏。

```text
POST /api/v1/chat/stream
Content-Type: application/json
{"user_id":"u1","session_id":"s1","query":"Redis 为什么超时？"}
```

```text
data: {"type":"content","content":"请"}
data: {"type":"content","content":"检查超时。"}
data: {"type":"done","usage":{"input_tokens":12,"output_tokens":4}}
```

## 事件契约

事件类型只有三种：

- `content`：`content` 是本次文本增量，不携带半成品消息。
- `done`：表示 assistant 已成功保存，可携带最终 `usage`。
- `error`：携带稳定的 `code` 和用户可读 `message`，随后关闭流。

正常示例：

```text
data: {"type":"content","content":"你好"}\n\n
data: {"type":"done","usage":{"input_tokens":12,"output_tokens":4}}\n\n
```

## 生命周期与失败策略

1. Handler 使用 `r.Context()` 调用 `Service.Stream`，不创建脱离请求的 `context.Background()`。
2. Service 先读取历史，再保存 user 消息，最后启动模型流。
3. 每次 `Recv` 只返回一个应用事件；空增量继续读取，不向客户端发送空事件。
4. 上游正常 EOF 时保存完整 assistant，返回一次 `done`；下一次 `Recv` 返回 `io.EOF`。
5. 上游错误、请求取消、超时、空响应或 assistant 保存失败，都不能保存半截 assistant。
6. Handler 必须 `defer stream.Close()`；`Close` 应幂等并关闭底层 `TokenStream`。

## 验收与测试

- 正常流：`content -> done -> EOF`，assistant 只保存一次。
- 上游错误：返回 `error`，只存在 user 消息。
- 客户端取消：下游 context 取消，底层流关闭，不保存 assistant。
- 空响应：返回 `EMPTY_RESPONSE`，不写 assistant。
- SSE 帧包含正确的 `data:` 前缀和两个换行，事件 JSON 不暴露供应商原始错误。
