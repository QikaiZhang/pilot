# build · 施工文档

本目录是 **施工规格**：开发时依据的结构、契约、数据模型、运行、测试与阶段目标。与 `docs/review/`（复盘与面试）和 `docs/history/`（训练变更记录）分开维护，避免施工依据和复习/面试材料混在一起。

推荐从 [project-contract.md](development/project-contract.md) 开始，再读[可施工规格](implementation/spec/README.md)。

## 目录

| 路径 | 内容 |
| --- | --- |
| [development](development/README.md) | 开发约定、目录职责、AI 协作规则 |
| [architecture](architecture/README.md) | 系统架构、数据流、中间件职责 |
| [api](api/README.md) | API 设计和联调清单 |
| [implementation](implementation/README.md) | 手写/AI 边界、提示词和代码分级；`spec/` 为可施工规格 |
| [testing](testing/README.md) | 测试策略、测试分层和故障演练 |
| [glossary](glossary/README.md) | 中间件、ES、RAG 等概念解释 |
| [stages](stages/README.md) | 阶段目标、实现顺序和验收门槛 |
