# 阶段开发规格

本目录描述每个阶段的目标、实现顺序和验收门槛，属于开发前的施工规格。实际完成情况、学习者答案和面试材料记录在根目录 `stages/`。

推荐按 10 个阶段推进，每阶段都只扩展一层能力。默认每阶段 1-2 周，按实际投入调整，不以日历时间作为硬门槛。

阶段文件已经按 `stage-00` 到 `stage-09` 编号，必须按数字顺序推进；Stage 09 不属于主业务闭环，放在单体验收之后。

| 阶段 | 目标 | 主要手写 | 通过标准 |
| --- | --- | --- | --- |
| [00 训练契约](stage-00-contract.md) | 明确范围、路线和数据流 | 项目边界、实体草图 | 能解释为什么先做单体 |
| [01 工程骨架](stage-01-skeleton.md) | 跑通 Go 服务和配置 | 配置校验、优雅关闭 | `/health` 可用，依赖检查清晰 |
| [02 记忆与持久化](stage-02-memory-persistence.md) | Redis + MySQL 双层记忆 | 窗口记忆合并、事务边界 | 聊天历史可保存和恢复 |
| [03 聊天 API](stage-03-chat-api.md) | HTTP、SSE、错误处理 | SSE 事件循环、取消传播 | 同步和流式接口都可测 |
| [04 ES 与 RAG](stage-04-es-rag.md) | 文档切分、向量、混合检索 | 检索过滤/排序和召回合并 | 能解释 ES 与 MySQL 的边界 |
| [05 Agent 与工具](stage-05-agent-tools.md) | Eino、ReAct、工具调用 | 工具路由和失败回退 | 一条工具链可观测、可重试 |
| [06 中间件与可观测性](stage-06-observability.md) | Metrics、Trace、Log | 请求拦截器的前后置逻辑 | Trace-ID、耗时和错误可定位 |
| [07 测试与交付](stage-07-test-delivery.md) | 集成测试、Compose、重构 | 关键链路测试和故障演练 | 可重复启动、测试和排障 |
| [08 面试交付](stage-08-interview.md) | 设计文档和口述 | 2 分钟项目介绍、深挖题 | 能讲清取舍、限制和优化 |
| [09 gRPC/微服务](stage-09-grpc-microservice.md) | 跨进程知识检索 | protobuf、client/server、超时、Trace 透传 | HTTP -> gRPC -> ES 链路可重复运行 |

每阶段的详细设计见同目录下的 `stage-*.md`。阶段间禁止跳过“设计 -> 手写 -> 测试 -> 复盘”中的任一项。Stage 09 必须在 Stage 07 通过后开始。
