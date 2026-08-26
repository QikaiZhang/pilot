# Stage 02：Redis 短期记忆与 MySQL 长期历史

## 目标

理解“快读写状态”和“可靠持久化”为什么需要不同存储，而不是把所有数据都放进 Redis 或 MySQL。

## 核心数据流

1. 请求携带 `user_id` 和 `session_id` 进入 Service。
2. Service 先从 Redis 读取最近 N 条消息。
3. Redis 未命中时回源 MySQL，读取成功后尝试回填 Redis。
4. 生成回复后，Service 先将消息批量写入 MySQL，再更新 Redis 窗口。
5. Redis 更新失败不撤销已成功的 MySQL 写入，记录告警并保留后续补偿空间。

历史查询和近期上下文是两个不同用例：近期上下文优先 Redis，完整历史使用 MySQL。Redis 只保存一个会话 key 下的独立消息 List，不是永久事实来源。

## 手写/AI 边界

- S：窗口裁剪、并发写入顺序、重复消息幂等键、事务边界。
- A：`Conversation`/`Message` 结构、DAO 和查询方法。
- B：Redis/MySQL 客户端初始化和重复 CRUD 样板。

## 必答问题

- Redis 为什么叫中间件，而不是业务模块？
- TTL、缓存击穿、写入失败和最终一致性如何处理？
- 同一个会话并发请求时，消息顺序如何保证？
- 为什么 MySQL 写入可以是 best-effort，但不能静默丢失？如何做重试或补偿？
