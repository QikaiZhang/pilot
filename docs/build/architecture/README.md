# 架构文档

推荐阅读顺序：

1. [system.md](system.md)：先看整体分层和启动顺序。
2. [chat-data-flow.md](chat-data-flow.md)：再看同步/SSE 的主链路和一致性。
3. [streaming-layers.md](streaming-layers.md)：理解 SSE 与 Stream 的三层转换和实例体现。
4. [storage-and-rag.md](storage-and-rag.md)：理解 Redis、MySQL、ES 的数据边界。
5. [agent-graph.md](agent-graph.md)：理解 Agent、RAG 和工具如何编排。
6. [middleware.md](middleware.md)：最后看横切能力如何包住主链路。

- [system.md](system.md)：总体架构、模块边界和核心数据流。
- [chat-data-flow.md](chat-data-flow.md)：同步/SSE 数据流、一致性与失败策略。
- [streaming-layers.md](streaming-layers.md)：三次流、两次转换、每层唯一转换点的代码级心智模型。
- [agent-graph.md](agent-graph.md)：Agent 图、RAG 编排、工具和多 Agent 边界。
- [middleware.md](middleware.md)：中间件模式、请求生命周期和可观测性。
- [storage-and-rag.md](storage-and-rag.md)：Redis/MySQL/ES 的职责和 RAG 数据流。
