# Pilot

Pilot 是一个面向运维排障的 Go AI 助手。它的目标是接收用户的问题，结合会话上下文、知识库和可观测性数据生成可追溯的排障建议。

> 当前状态：Stage 01-03 已完成；Stage 04 已完成 ES 混合检索基础，正在补知识文档导入、版本管理和召回评估；Stage 05 已接通 Eino Agent、health_check、knowledge_search、手写 Loop 基准和 ToolCall 审计。Prometheus/Trace/Log 观测安排在 Agent/RAG 业务事件稳定后接入。

## 系统全览

```text
客户端
  -> HTTP/SSE API
  -> Handler
  -> 会话记忆（Redis）
  -> Agent / 模型 / 工具
  -> 历史事实（MySQL）
  -> 知识库导入/检索（Elasticsearch）
  -> 指标、日志、Trace
```

各层职责如下：

- `cmd/`：程序入口和运维命令。
- `internal/handler/`：HTTP Handler、请求校验和响应编码，包括 Agent 与知识文档导入入口。
- `internal/deps/`：启动时检查外部依赖。
- `internal/agent/`：Agent Runner、执行策略和编排流程。
- `internal/tools/`：Agent 可调用工具及 Registry。
- `internal/ai/`：模型契约、供应商适配、Prompt、Embedding、知识切分和检索。
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

当前会话逻辑已接入同步 Chat、SSE 和 Agent 路由。Redis 保存最近窗口，MySQL 保存完整历史；Agent 请求还会在需要时调用知识库工具。

## 本地开发

```bash
make test       # 运行根模块全部测试
make run        # 启动 Pilot
make build      # 构建 bin/pilot
make compose-up # 启动 Redis/MySQL/Elasticsearch 等依赖
make health     # 检查 live/ready 接口
```

依赖配置可放入本地 `.env`，不要提交真实密钥或连接凭据。建议先阅读 [开发文档](docs/build/development/README.md)，再查看 [可施工规格](docs/build/implementation/spec/README.md) 和 [阶段路线](docs/build/stages/README.md)。

## 当前学习路线

1. Stage 01：启动、配置、健康检查和依赖边界。
2. Stage 02：Redis 短期记忆与 MySQL 长期历史。
3. Stage 03：同步聊天、SSE、取消传播和错误处理。
4. Stage 04-05 收尾：知识导入、RAG 评估、Agent 工具限制、失败回退和审计。
5. Stage 06：在上述业务事件稳定后接入 Prometheus、Trace 和结构化日志。
