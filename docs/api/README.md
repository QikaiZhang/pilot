# API 文档

阅读顺序：

1. [contract.md](contract.md)：先记住统一响应、错误码和安全边界。
2. 按阶段查看接口：Stage 01 健康检查，Stage 02 历史，Stage 03 Chat/SSE，Stage 04 知识库，Stage 06 可观测性。
3. 联调时只使用本目录的请求/响应契约，不直接从实现猜字段。

## 基础约定

- 前缀：`/api/v1`。
- 响应统一包含 `code`、`message`、`data`、`trace_id`。
- 所有写接口都要记录幂等键或明确说明不支持幂等。
- 错误响应不能泄露模型密钥、数据库连接串或内部堆栈。

## 当前接口清单

完整错误码、请求结构和 SSE 协议见 [contract.md](contract.md)。本页保留阶段导航。

| 方法 | 路径 | 用途 | 阶段 |
| --- | --- | --- | --- |
| GET | `/api/v1/health/live` | 进程存活检查 | 01 |
| GET | `/api/v1/health/ready` | 依赖就绪检查 | 01 |
| POST | `/api/v1/chat` | 同步对话（主路径） | 03 |
| POST | `/api/v1/chat/` | 同步对话（旧客户端兼容） | 03 |
| POST | `/api/v1/chat/stream` | SSE 流式对话 | 03 |
| GET | `/api/v1/chat/history` | 历史查询 | 02/03 |
| DELETE | `/api/v1/chat/history` | 历史清理 | 02/03 |
| POST | `/api/v1/chat/multi-agent` | 多 Agent 排障 | 05 |
| POST | `/api/v1/knowledge/upload` | 文档导入和索引 | 04 |
| POST | `/api/v1/knowledge/search` | 知识检索 | 04 |
| POST | `/api/v1/observability/overview` | 观测概览 | 06 |

## SSE 事件建议

事件类型至少包括 `start`、`token`、`tool`、`error`、`done`。每个事件带 `conversation_id` 和 `trace_id`；客户端断开后服务端必须停止继续调用模型。

## API 来源

课件提供了 Apifox 参考链接，但本仓库只沉淀接口契约和学习用示例，不把外部访问密码写入代码或文档。
