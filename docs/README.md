# Pilot 学习型项目文档

这组文档是 Pilot 的项目说明、开发规格和学习入口。目标是通过一条可运行的数据流熟悉 Go 项目开发过程，再逐步加入 Redis、MySQL、Elasticsearch、Agent、可观测性和微服务边界。

## 文档分工

`docs/` 顶层只保留三块，施工与复盘彻底分开，便于后续按用途检索：

| 目录 | 用途 |
| --- | --- |
| [build/](build/README.md) | **施工**：开发、架构、API、测试、实现规格、阶段目标（应做什么） |
| [review/](review/README.md) | **复盘与面试**：思考题、面试表达、设计复盘（学习者后续使用） |
| [history/](history/README.md) | **历史记录**：训练变更记录（changelog） |

根目录 `stages/`（`CHANGELOG.md`、`THOUGHT.md`、各阶段子目录）是本次实际训练产物；`源代码/` 是本地参考资料，已被 Git 忽略，不属于当前实现。

## 01. 推荐阅读顺序

按下面顺序阅读和开发，不需要先看源码：

| 顺序 | 先读 | 目的 |
| ---: | --- | --- |
| 1 | [训练契约](build/development/project-contract.md) | 明确范围、手写边界和完成规则 |
| 2 | [可施工规格](build/implementation/spec/README.md) | 准备环境、数据模型、API 和 Mock 模型 |
| 3 | [开发约定](build/development/README.md) 与 [手写/AI 规则](build/implementation/README.md) | 确定目录、提示词和协作方式 |
| 4 | [阶段总览](build/stages/README.md) 与 `stage-00` | 了解每阶段目标和验收门槛 |
| 5 | [总体架构](build/architecture/system.md) | 建立模块和依赖的全局心智模型 |
| 6 | `stage-01` 到 `stage-07` | 逐层实现单体闭环，每阶段完成后再继续 |
| 7 | [测试策略](build/testing/README.md) 与 `stage-08` | 做真实链路验收和面试沉淀 |
| 8 | [Stage 09](build/stages/stage-09-grpc-microservice.md) | 最后练习跨进程 gRPC/微服务打通 |

每个阶段固定执行：**预读伴随文档 -> 写设计草图 -> 手写 S 级逻辑 -> 生成重复样板 -> 测试 -> 更新 `THOUGHT.md`**。

## 02. 文档导航

| 目录 | 用途 |
| --- | --- |
| [build/development](build/development/README.md) | 开发约定、目录职责、AI 协作规则 |
| [build/stages 规格](build/stages/README.md) | 分阶段目标、实现顺序和验收门槛 |
| [build/architecture](build/architecture/README.md) | 系统架构、数据流、中间件职责 |
| [build/api](build/api/README.md) | API 设计和联调清单 |
| [build/implementation](build/implementation/README.md) | 手写/AI 边界、提示词和代码分级 |
| [build/testing](build/testing/README.md) | 测试策略、测试分层和故障演练 |
| [review/interview](review/interview/README.md) | 面试沉淀和口述训练 |
| [review/THOUGHT](review/THOUGHT.md) | 设计复盘与评分记录 |
| [history/CHANGELOG](history/CHANGELOG.md) | 训练变更记录 |
| [build/glossary](build/glossary/README.md) | 中间件、ES、RAG 等概念解释 |
| [训练契约](build/development/project-contract.md) | 项目范围、手写边界和学习规则 |
| [重复逻辑练习台](build/implementation/repetition-lab.md) | 每类重复代码至少手写一个样例 |
| [可施工规格](build/implementation/spec/README.md) | 删除源码后仍可执行的环境、数据、API 和 Mock 规格 |

## 03. 阶段伴读文档

| 阶段 | 实现前阅读 |
| --- | --- |
| 01 工程骨架 | `build/implementation/spec/01-environment.md`、`build/architecture/system.md`、`build/api/contract.md` |
| 02 记忆持久化 | `build/implementation/spec/02-data-model.md`、`build/architecture/storage-and-rag.md`、`build/architecture/chat-data-flow.md` |
| 03 聊天与 SSE | `build/implementation/spec/03-api-schemas.md`、`build/architecture/chat-data-flow.md` |
| 04 ES/RAG | `build/implementation/spec/02-data-model.md`、`build/architecture/storage-and-rag.md` |
| 05 Agent/工具 | `build/architecture/agent-graph.md`、`review/interview/thought-questions.md` |
| 06 可观测性 | `build/architecture/middleware.md`、`build/testing/README.md` |
| 07 测试交付 | `build/testing/README.md`、`build/implementation/spec/01-environment.md` |
| 08 面试沉淀 | `review/interview/README.md`、`review/interview/thought-questions.md` |
| 09 gRPC/微服务 | [stage-09-grpc-microservice.md](build/stages/stage-09-grpc-microservice.md) 及其目录 [README](build/stages/stage-09-grpc-microservice/README.md) |

## 当前项目画像

- 项目：Pilot 磐石，面向运维场景的 AI 助手。
- 本地仓库现状：源码已作为设计参考放入仓库；学习过程以本 `docs/` 为唯一入口。
- 目标：熟悉 Go 项目的设计、编码、测试、部署和复盘闭环。
- 学习者基础：熟悉 Redis、MySQL；参与过中小型 Go 项目；Go 基础、gRPC、微服务和 ES 需要补强。
- 默认训练方式：路线 A（AI 提供骨架，核心逻辑由学习者手写），重复性代码只手写 2-3 个代表样例，其余由脚手架生成后阅读和 Review。

## 参考来源

- 飞书课件：[Pilot 磐石项目课件](https://q0kyes8lnt5.feishu.cn/wiki/KQRIwrpL0iAImjk5zzFcpyOSnde)
- 课件中的代码、八股和 API 文档作为参考资料，不直接复制成最终答案。

## 使用方式

每进入一个阶段，先阅读对应的开发规格和阶段文档，完成设计问题，再开始编码。阶段未通过验收时，不扩展下一层技术。实际进展统一记录在 `stages/CHANGELOG.md`、`stages/THOUGHT.md` 和对应的 `stages/stage-XX/` 中。
