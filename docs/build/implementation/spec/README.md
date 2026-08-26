# 可施工规格

本目录是删除源码后仍可执行项目的最低规格。文件名已经按阅读顺序编号：环境启动 -> 数据模型 -> API Schema -> Mock 模型 -> 真实模型替换 -> 流式输出。

| 文档 | 用途 |
| --- | --- |
| [01-environment.md](01-environment.md) | 工具、端口、环境变量、启动和健康检查 |
| [02-data-model.md](02-data-model.md) | MySQL、Redis、ES 的字段和索引约定 |
| [03-api-schemas.md](03-api-schemas.md) | 请求、响应、分页和 SSE 示例 |
| [04-local-model.md](04-local-model.md) | 不依赖外部模型完成全链路 |
| [05-memory-persistence.md](05-memory-persistence.md) | Redis 短期窗口、MySQL 历史与失败策略 |
| [06-streaming-sse.md](06-streaming-sse.md) | Chat Stream 生命周期、SSE 事件和取消策略 |

## 使用规则

每完成一个规格，先写一个最小测试或 curl 验证，再进入下一个规格。规格中的 `MUST` 是契约；实现可以换库或换目录，但不能改变外部行为。
