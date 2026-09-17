# Redis 内存淘汰与 maxmemory 排查

## 现象

Redis 写入报错 OOM command not allowed when used memory > maxmemory，或出现部分 key 被静默删除、缓存命中率下降。

## 排查步骤

先查 INFO memory 中 used_memory 与 maxmemory 的水位，确认是否触顶；再用大 key 扫描定位膨胀 key，检查过期策略与业务写入增长。确认 maxmemory-policy 配置：noeviction 在写满后直接拒绝写入，allkeys-lru 会按 LRU 淘汰旧 key。

## 处理建议

按业务容忍度选择淘汰策略：可重建的缓存场景用 allkeys-lru，强一致数据不能只存 Redis。清理或拆分大 key，为 key 设置合理 TTL，必要时扩容内存或迁移到集群分片。
