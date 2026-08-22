# Pilot 学习型项目文档

这组文档是 Pilot 的项目说明、开发规格和学习入口。目标是通过一条可运行的数据流熟悉 Go 项目开发过程，再逐步加入 Redis、MySQL、Elasticsearch、Agent、可观测性和微服务边界。

## 文档分工

项目文档分成两条轨道，但不复制同一份内容：

| 轨道 | 路径 | 用途 |
| --- | --- | --- |
| 开发文档 | `docs/development/`、`docs/architecture/`、`docs/api/`、`docs/implementation/`、`docs/testing/` | 当前代码的结构、契约、数据模型、运行和测试依据 |
| 学习文档 | `docs/stages/`、`docs/interview/`、`stages/` | 阶段目标、训练记录、复盘问题、面试表达和验收证据 |

其中，`docs/stages/` 描述“阶段应该做什么”，`stages/` 记录“这次实际做了什么”。根目录 `README.md` 只保留系统全览和快速入口；`源代码/` 是本地参考资料，已被 Git 忽略，不属于当前实现。

## 01. 推荐阅读顺序

按下面顺序阅读和开发，不需要先看源码：

| 顺序 | 先读 | 目的 |
| ---: | --- | --- |
| 1 | [训练契约](development/project-contract.md) | 明确范围、手写边界和完成规则 |
| 2 | [可施工规格](implementation/spec/README.md) | 准备环境、数据模型、API 和 Mock 模型 |
| 3 | [开发约定](development/README.md) 与 [手写/AI 规则](implementation/README.md) | 确定目录、提示词和协作方式 |
| 4 | [阶段总览](stages/README.md) 与 `stage-00` | 了解每阶段目标和验收门槛 |
| 5 | [总体架构](architecture/system.md) | 建立模块和依赖的全局心智模型 |
| 6 | `stage-01` 到 `stage-07` | 逐层实现单体闭环，每阶段完成后再继续 |
| 7 | [测试策略](testing/README.md) 与 `stage-08` | 做真实链路验收和面试沉淀 |
| 8 | [Stage 09](stages/stage-09-grpc-microservice.md) | 最后练习跨进程 gRPC/微服务打通 |

每个阶段固定执行：**预读伴随文档 -> 写设计草图 -> 手写 S 级逻辑 -> 生成重复样板 -> 测试 -> 更新 `THOUGHT.md`**。

## 02. 文档导航

| 目录 | 用途 |
| --- | --- |
| [development](development/README.md) | 开发约定、目录职责、AI 协作规则 |
| [stages 规格](stages/README.md) | 分阶段目标、实现顺序和验收门槛 |
| [architecture](architecture/README.md) | 系统架构、数据流、中间件职责 |
| [api](api/README.md) | API 设计和联调清单 |
| [implementation](implementation/README.md) | 手写/AI 边界、提示词和代码分级 |
| [testing](testing/README.md) | 测试策略、测试分层和故障演练 |
| [interview](interview/README.md) | 面试沉淀和口述训练 |
| [glossary](glossary/README.md) | 中间件、ES、RAG 等概念解释 |
| [训练契约](development/project-contract.md) | 项目范围、手写边界和学习规则 |
| [重复逻辑练习台](implementation/repetition-lab.md) | 每类重复代码至少手写一个样例 |
| [可施工规格](implementation/spec/README.md) | 删除源码后仍可执行的环境、数据、API 和 Mock 规格 |

## 03. 阶段伴读文档

| 阶段 | 实现前阅读 |
| --- | --- |
| 01 工程骨架 | `spec/01-environment.md`、`architecture/system.md`、`api/contract.md` |
| 02 记忆持久化 | `spec/02-data-model.md`、`architecture/storage-and-rag.md`、`architecture/chat-data-flow.md` |
| 03 聊天与 SSE | `spec/03-api-schemas.md`、`architecture/chat-data-flow.md` |
| 04 ES/RAG | `spec/02-data-model.md`、`architecture/storage-and-rag.md` |
| 05 Agent/工具 | `architecture/agent-graph.md`、`interview/thought-questions.md` |
| 06 可观测性 | `architecture/middleware.md`、`testing/README.md` |
| 07 测试交付 | `testing/README.md`、`spec/01-environment.md` |
| 08 面试沉淀 | `interview/README.md`、`interview/thought-questions.md` |
| 09 gRPC/微服务 | [stage-09-grpc-microservice.md](stages/stage-09-grpc-microservice.md) 及其目录 [README](stages/stage-09-grpc-microservice/README.md) |

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
