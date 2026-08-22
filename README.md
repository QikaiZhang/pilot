# Pilot

Pilot 是一个面向运维排障的 Go AI 助手。它的目标是接收用户的问题，结合会话上下文、知识库和可观测性数据生成可追溯的排障建议。

> 当前状态：Stage 01 已完成基础骨架，Stage 02 已完成消息模型、MySQL 历史、Redis 短期记忆和 `memory.Service` 的核心读写协调。聊天 API、AI 模型、知识检索和 Agent 仍按阶段实现中。

## 系统全览

```text
客户端
  -> HTTP/SSE API
  -> Controller
  -> 会话记忆（Redis）
  -> Agent / 模型 / 工具
  -> 历史事实（MySQL）
  -> 知识检索（Elasticsearch）
  -> 指标、日志、Trace
```

各层职责如下：

- `cmd/`：程序入口和运维命令。
- `internal/controller/`：HTTP 接口和请求校验。
- `internal/deps/`：启动时检查外部依赖。
- `pkg/`：配置、环境变量等可复用工具。
- `manifest/`：Docker Compose 和运行配置。
- `docs/`、`stages/`：设计契约、学习路线和阶段验收。
- `源代码/`：未纳入根模块构建的参考实现，不是当前运行代码。

## 什么是“会话”

会话（session）就是一条连续的对话线程。例如用户 `alice` 可以有两个会话：一个讨论 Redis 超时，另一个讨论 MySQL 慢查询。它们拥有不同的 `session_id`，上下文和历史互不混淆。

请求使用 `user_id` 表示谁在提问，使用 `session_id` 表示哪条对话线程：

```json
{"user_id":"alice","session_id":"redis-timeout-1","query":"最近的错误是什么？"}
```

同一会话的后续请求继续使用 `redis-timeout-1`，系统才能读取之前的消息。Redis 保存最近窗口，MySQL 保存完整历史；`session_id` 不是登录凭证，也不是独立用户体系。

目前根代码还没有实现聊天路由，因此会话逻辑现在是 API 和持久化设计中的概念，而不是已经可调用的完整功能。Stage 02 会实现历史和记忆存储，Stage 03 再接入同步聊天与 SSE。

## 本地开发

```bash
make test       # 运行根模块全部测试
make run        # 启动 Pilot
make build      # 构建 bin/pilot
make compose-up # 启动 Redis/MySQL/Elasticsearch 等依赖
make health     # 检查 live/ready 接口
```

依赖配置可放入本地 `.env`，不要提交真实密钥或连接凭据。建议先阅读 [开发文档](docs/development/README.md)，再查看 [可施工规格](docs/implementation/spec/README.md) 和 [阶段路线](docs/stages/README.md)。

## 当前学习路线

1. Stage 01：启动、配置、健康检查和依赖边界。
2. Stage 02：Redis 短期记忆与 MySQL 长期历史。
3. Stage 03：同步聊天、SSE、取消传播和错误处理。
4. 后续阶段：知识检索、Agent 工具、多 Agent 和可观测性。
