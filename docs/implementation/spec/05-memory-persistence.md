# 记忆与历史持久化规格

## 1. 目标与边界

Stage 02 为 Pilot 增加两层会话数据存储：Redis 保存可过期的近期上下文，MySQL 保存完整、可靠的会话历史。Redis 是加速层，不是事实来源；MySQL 是历史查询和恢复窗口的事实来源。

本阶段只实现会话消息，不实现跨会话的 episodic memory、摘要生成或向量检索。`源代码/` 下的实现只用于对照行为，根模块必须依据本文档和测试重新实现。

## 2. 领域模型

业务请求使用 `user_id` 和 `session_id` 标识会话。现有 API 文档中的 `conversation_id` 是外部命名；进入领域层后必须统一映射为 `session_id`，不能在存储层同时维护两套标识。每条消息至少包含：

```go
type ChatMessage struct {
    ID        string
    UserID    string
    SessionID string
    Role      string
    Content   string
    Metadata  map[string]any
    CreatedAt time.Time
}
```

`Role` 只允许 `user`、`assistant`、`system`、`tool`。`ID` 由服务端或请求方提供的幂等标识生成；同一个消息 ID 在同一会话中只能成功持久化一次。

MySQL 使用 `conversation_history` 表，唯一事实排序为 `(created_at, id)` 升序。Redis 使用逻辑 key `pilot:memory:{user_id}:{session_id}`，保存最近 `REDIS_MAX_MESSAGES` 条 JSON 消息。Redis key 的用户输入必须经过长度限制和转义，不能让用户构造任意 key 空间。

## 3. 写入与读取数据流

### 3.1 读取上下文

1. 校验 `user_id`、`session_id` 和请求参数。
2. 从 Redis 读取最近窗口。
3. Redis 命中时直接提供给 Agent；Redis 未命中时从 MySQL 按 `(created_at, id)` 读取最近窗口，并尝试回填 Redis。
4. Redis 连接错误默认返回明确错误，不把“依赖不可用”伪装成空上下文。若未来启用降级模式，必须记录降级原因和 trace ID。

### 3.2 保存消息

一次聊天至少保存用户消息和助手消息。每条消息先在 MySQL 事务中写入，再更新 Redis 窗口。MySQL 写入失败时请求失败，不能返回看似成功的响应。MySQL 成功而 Redis 失败时，历史仍然可靠；请求可以成功，但必须记录 Redis 错误，下一次读取可从 MySQL 重建窗口。

Redis 追加必须以一个原子操作完成：

```text
RPUSH -> LTRIM(保留最新 N 条) -> EXPIRE
```

不能用多个无保护的网络请求替代。超过 `REDIS_CHUNK_BYTES` 时才使用 `:chunk:{n}` 分片；分片读取按数字序号合并，删除使用 `SCAN`，禁止生产代码使用 `KEYS`。

## 4. 一致性、并发与幂等

同一 `session_id` 的并发请求必须保持确定顺序。第一版可以使用按会话分片的进程内互斥锁，保护“读取窗口、追加、裁剪”这一整个临界区；锁不能覆盖网络等待以外的无关业务，也不能被误认为跨实例锁。多实例部署前必须升级为 Redis 分布式锁、数据库序列号或消息队列方案，并补充租约失效设计。

MySQL 为消息 ID 建立唯一约束。重试遇到唯一键冲突时，应读取并复用已存在的消息，而不是再次追加。事务边界必须覆盖同一次请求产生的用户消息和助手消息，不能只保存其中一条。历史删除必须同时带上 `user_id` 和 `session_id`，避免越权删除。

## 5. 失败策略

| 场景 | 行为 | 必须观察的结果 |
| --- | --- | --- |
| Redis 读取失败 | 默认失败；降级模式才允许空上下文或 MySQL 回退 | 错误日志、trace、降级计数 |
| Redis 未命中 | 读 MySQL 最近窗口并回填 | 回填成功/失败 |
| Redis 写入失败、MySQL 成功 | 保留成功响应，记录错误 | Redis 错误指标 |
| MySQL 写入失败 | 请求失败，不返回已保存语义 | 事务回滚、明确错误码 |
| 同一消息重复提交 | 唯一键幂等，返回原消息结果 | 重复计数或日志 |
| 删除部分分片失败 | 返回错误，不报告删除完成 | 未删除 key 数量 |

## 6. 接口与实现边界

建议先定义与存储无关的接口：

```go
type HistoryStore interface {
    Append(ctx context.Context, messages []ChatMessage) error
    List(ctx context.Context, userID, sessionID string, limit int) ([]ChatMessage, error)
    Delete(ctx context.Context, userID, sessionID string) error
}

type RecentMemory interface {
    Get(ctx context.Context, userID, sessionID string, limit int) ([]ChatMessage, error)
    Append(ctx context.Context, messages []ChatMessage, limit int, ttl time.Duration) error
    Clear(ctx context.Context, userID, sessionID string) error
}
```

Repository 负责查询和事务，Redis adapter 负责 key、序列化、TTL 和原子脚本；Controller 不应直接拼 SQL 或 Redis 命令。配置沿用 `pkg/configutil` 的 Redis/MySQL 字段，真实客户端在启动阶段初始化并接受依赖注入。

## 7. 测试与阶段验收

先使用 fake HistoryStore 和 fake RecentMemory 验证业务时序，再补真实依赖测试。必须覆盖：窗口裁剪、TTL 参数、Redis 未命中回源、同会话并发顺序、重复消息幂等、MySQL 事务回滚、按用户和会话删除、Redis/MySQL 各自不可用时的错误策略。最终在 Compose 中执行健康检查、聊天写入、历史查询、Redis 重启后的 MySQL 回源验证。

## 8. 学习与复盘问题

- 为什么 Redis 写失败可以在 MySQL 成功后降级，而 MySQL 写失败不能静默降级？
- `RPUSH`、`LTRIM`、`EXPIRE` 分成三次请求时，具体会产生哪些竞态？
- 进程内锁在单实例和多实例部署中的边界分别是什么？
- 幂等 ID 应由客户端生成还是服务端生成？重试时如何证明两次请求是同一条消息？
- 为什么历史查询必须按 `(created_at, id)`，只按时间排序有什么问题？
