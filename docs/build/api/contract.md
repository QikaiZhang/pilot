# API 契约

## 统一响应

除已经建立的 SSE 连接外，接口使用统一 JSON envelope：

```json
{"success":true,"code":"OK","message":"对话成功","data":{}}
```

失败响应不返回内部堆栈、模型密钥或连接串。

| code | HTTP | 含义 |
| --- | ---: | --- |
| `INVALID_ARGUMENT` | 400 | 请求体、查询参数或过滤条件不合法 |
| `UNAUTHENTICATED` | 401 | API Key 缺失或错误 |
| `NOT_FOUND` | 404 | 路由或资源不存在 |
| `FEATURE_DISABLED` | 404 | 可选功能被关闭 |
| `RATE_LIMITED` | 429 | 超过 IP 固定窗口限制 |
| `UPSTREAM_ERROR` | 502 | 模型或 Agent 上游失败 |
| `DEPENDENCY_UNAVAILABLE` | 503 | Redis、MySQL、ES 或观测数据源不可用 |
| `INTERNAL` | 500 | 未分类服务端异常 |

## 接口清单

| 方法 | 路径 | 用途 | 阶段 |
| --- | --- | --- | --- |
| GET | `/api/v1/health` | 返回健康摘要 | 01 |
| GET | `/api/v1/health/live` | 进程存活检查 | 01 |
| GET | `/api/v1/health/ready` | Redis/MySQL/ES 就绪检查 | 01 |
| POST | `/api/v1/chat` | 同步对话 | 03 |
| POST | `/api/v1/chat/` | 旧客户端兼容路径 | 03 |
| POST | `/api/v1/chat/stream` | SSE 流式对话 | 03 |
| POST | `/api/v1/agent/chat` | 带工具编排的同步 Agent 对话 | 05 |
| POST | `/api/v1/knowledge/documents` | 导入知识文档并切分入库 | 05 |
| POST | `/api/v1/chat/multi-agent` | 多 Agent 排障 | 05 |
| GET | `/api/v1/chat/history` | 查询会话历史 | 02 |
| DELETE | `/api/v1/chat/history` | 清理会话历史 | 02 |
| POST | `/api/v1/knowledge/upload` | 导入知识文档 | 04 |
| POST | `/api/v1/knowledge/search` | 查询知识库 | 04 |
| POST | `/api/v1/knowledge/search/detailed` | 查询各路召回分数 | 04 |
| DELETE | `/api/v1/knowledge?id=...` | 删除知识文档 | 04 |
| POST | `/api/v1/observability/alerts` | 查询告警 | 06 |
| POST | `/api/v1/observability/metrics` | 查询指标 | 06 |
| POST | `/api/v1/observability/logs` | 查询运行日志 | 06 |
| POST | `/api/v1/observability/overview` | 聚合观测摘要 | 06 |

## Chat 请求

```json
{"user_id":"user-1","session_id":"session-1","query":"排查最近的错误"}
```

## SSE 约定

- 响应类型：`text/event-stream`。
- 普通分片：`data: 文本\n\n`。
- 正常结束：`data: [DONE]\n\n`。
- 流中断：发送包含稳定错误码的 `data: {"error":{...}}` 后关闭连接。
- 客户端断开后，服务端必须停止继续调用模型并释放 stream。

## 安全边界

开启鉴权后支持 `Authorization: Bearer <key>` 或 `X-API-Key`。健康接口和 `/metrics` 豁免。它是共享服务 API Key，不是用户体系或多租户 IAM。
