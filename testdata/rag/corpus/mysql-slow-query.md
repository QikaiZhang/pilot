# MySQL 慢查询定位与优化

## 现象

应用响应变慢，数据库 CPU 或 IO 升高，SHOW PROCESSLIST 中出现长时间处于 Sending data 或 Creating sort index 状态的查询。

## 排查步骤

开启 slow_query_log 并按 long_query_time 过滤慢 SQL；对目标语句执行 EXPLAIN，关注 type 是否为 ALL 全表扫描、key 是否为 NULL、rows 扫描行数，以及 Extra 中的 Using filesort、Using temporary。结合业务确认查询模式是否命中索引。

## 处理建议

为过滤列与排序列建立联合索引，避免在索引列上使用函数或隐式类型转换；大偏移分页改为游标方式；必要时读写分离或引入缓存。上线前用慢查询日志回归验证优化效果。
