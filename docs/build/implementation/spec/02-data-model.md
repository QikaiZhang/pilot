# 数据模型规格

## MySQL：conversation_history

```sql
CREATE TABLE conversation_history (
  id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
  message_id VARCHAR(128) NOT NULL,
  user_id VARCHAR(128) NOT NULL,
  session_id VARCHAR(128) NOT NULL,
  role VARCHAR(16) NOT NULL,
  content LONGTEXT NOT NULL,
  metadata JSON NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uk_history_message (user_id, session_id, message_id),
  KEY idx_history_session_created (user_id, session_id, created_at, id)
);
```

`id` 是数据库内部主键；`message_id` 是请求级幂等键，在同一用户和会话中唯一。`role` 只允许 `user`、`assistant`、`system`、`tool`。历史查询按 `created_at, id` 升序返回，删除必须同时要求 `user_id` 和 `session_id`，防止误删其他会话。

## MySQL：episodic_incidents

用于跨会话排障摘要，不作为实时指标事实来源：`service`、`intent`、`query`、`answer`、`evidence_ids`（JSON 数组）、`limitations`（JSON 数组）、`created_at`。Stage 05 之后再实现，前面的阶段可以不建表。

## Redis：短期窗口

逻辑 key 固定为 `pilot:memory:{user_id}:{session_id}`，每个元素是一个 JSON 消息：

```json
{"role":"user","content":"最近错误是什么","created_at":"2026-01-01T00:00:00Z"}
```

追加使用单次事务或 Lua 保证 `RPUSH -> LTRIM -> EXPIRE` 的顺序；窗口保留最新 `REDIS_MAX_MESSAGES` 条。序列化后超过 `REDIS_CHUNK_BYTES` 时，按 `:chunk:{n}` 分片，读取按 chunk 序号合并；删除使用 `SCAN`，禁止生产代码用 `KEYS`。

## Elasticsearch：pilot_knowledge

```json
{
  "mappings": {
    "properties": {
      "doc_id": {"type":"keyword"},
      "chunk_id": {"type":"keyword"},
      "title": {"type":"text", "fields":{"raw":{"type":"keyword"}}},
      "content": {"type":"text"},
      "category": {"type":"keyword"},
      "tags": {"type":"keyword"},
      "source": {"type":"keyword"},
      "version": {"type":"integer"},
      "embedding_model": {"type":"keyword"},
      "embedding_dim": {"type":"integer"},
      "vector": {"type":"dense_vector", "dims":32, "index":true, "similarity":"cosine"},
      "created_at": {"type":"date"}
    }
  }
}
```

`dims` 必须和 `EMBEDDING_DIM` 一致。模型或维度变化时新建版本索引、批量重建、切换 alias，不能在同一个向量字段混写不同维度。

## 一致性规则

- MySQL 是会话历史事实来源。
- Redis 是可过期的加速上下文，丢失后可从 MySQL 重建窗口。
- ES 是可重建的搜索索引，上传成功与索引成功要分别记录状态。
