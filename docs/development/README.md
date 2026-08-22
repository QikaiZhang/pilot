# 开发文档

本目录面向“把代码写出来并能运行”的开发过程：

1. [project-contract.md](project-contract.md)：先确定学习范围和手写边界。
2. 本页：再确定目录、分支和本地开发规则。
3. [实现规格](../implementation/spec/README.md)：准备可运行环境和数据契约。
4. 开发完成后，再到 [阶段规格](../stages/README.md) 和根目录 `stages/` 做复盘，不把学习笔记混入实现约定。

## 1. 开发原则

1. 先设计后编码：先画数据流、确定字段和错误路径，再写代码。
2. 先单体后拆服务：Pilot 先作为一个 Go 服务跑通，只有边界稳定后才演练 gRPC/微服务。
3. 先可读后抽象：优先直白的 Go 写法，重复 2-3 次后再提取公共组件。
4. 一条链路优先：先跑通“聊天请求 -> 记忆 -> Agent -> 响应 -> 历史落库”，再加更多工具。
5. 依赖显式失败：Redis、MySQL、ES 等强依赖在启动时检查，不能用运行时隐式失败替代启动检查。

## 2. 目录职责

```text
cmd/             程序入口和运维命令
internal/
  controller/    HTTP/SSE 接口层
  memory/        Redis 短期记忆、MySQL 长期历史
  ai/            Agent、模型、Prompt、Embedding、检索、工具
  observability/ 指标、Trace、结构化日志中间件
pkg/             可复用的配置、Redis、ES、环境变量和类型工具
manifest/        YAML、Docker Compose 等交付配置
test/            单元、集成和端到端测试
web/             极简前端工作台，由 Go 直接托管
```

## 3. 分支和阶段

`main` 只保存已验收版本；每阶段从上一阶段分支创建 `stage/XX-name`。阶段完成要更新 `CHANGELOG.md`、`THOUGHT.md`，运行测试并做一次口述验收。

实际训练记录位于根目录 `stages/`；`docs/stages/` 只描述阶段目标和验收标准。

## 4. 本地启动约定

- 敏感配置只放 `.env`，提交 `.env.example`，不提交真实密钥。
- 依赖优先通过 Docker Compose 启动，避免把本机环境差异带进学习结果。
- 每次新增依赖都记录：解决什么问题、替代方案、失败时如何降级。

## 5. 施工入口

删除源码后，先阅读 [可施工规格](../implementation/spec/README.md)，再按 [分阶段计划](../stages/README.md) 开始。遇到外部模型、ES 或微服务概念时，优先使用 Mock 和当前阶段的最小契约，不要提前扩展范围。
