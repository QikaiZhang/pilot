# API Schema 规格

所有 JSON 接口使用：

```json
{"success":true,"code":"OK","message":"ok","data":{}}
```

## Chat

请求：

```json
{"user_id":"u-1","session_id":"s-1","query":"排查最近错误"}
```

约束：`user_id` 和 `session_id` 非空、最长 128；`query` 非空、最长 8000；服务端 trim 后再进入 Agent。

同步成功 `data`：

```json
{"answer":"...","session_id":"s-1","message_id":"m-1","trace_id":"t-1","citations":[],"limitations":[]}
```

同步失败只返回稳定错误码和用户可读 message，不返回模型原始响应、SQL、连接串或内部堆栈。

## Chat History

请求参数：`user_id`、`session_id`、`limit`（默认 20，最大 100）、`before`（可选 ISO 时间）。`data`：

```json
{"items":[{"id":"m-1","role":"user","content":"...","created_at":"2026-01-01T00:00:00Z"}],"next_before":"2026-01-01T00:00:00Z"}
```

## Knowledge

上传请求：`title`、`content`、`category`、`tags[]`、`source`、`version`。成功返回 `doc_id`、`chunk_count`、`embedding_model`、`embedding_dim`。

搜索请求：`query`、`category`（可选）、`tags[]`（可选）、`top_k`（默认 5，最大 20）、`mode`（`hybrid`、`bm25`、`vector`）。结果至少包含 `doc_id`、`chunk_id`、`title`、`content`、`score`、`source`。

## Observability

四个查询接口都接受 `from`、`to`、`service`、`limit`；`overview` 返回 `metrics`、`logs`、`traces` 和 `limitations`。数据源部分不可用时，HTTP 仍可成功，但必须将对应数据源写入 `limitations`。

## SSE

```text
Content-Type: text/event-stream
data: {"delta":"你好"}\n\n
data: {"delta":"，世界"}\n\n
data: [DONE]\n\n
```

流错误使用 `data: {"error":{"code":"UPSTREAM_ERROR","message":"..."}}`，随后关闭连接。客户端断开必须取消同一个 `context.Context`。
