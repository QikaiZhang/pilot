# Stage 01 学习沉淀目录

> 目的：每个阶段把「高价值代码块」写成**可沉淀的模板文档 + 面试整理**，方便复盘与口述训练。本目录是 Stage 01（工程骨架）的对应产出，属于 `stages/` 阶段实训体系。

## 目录

| 文件 | 用途 |
| --- | --- |
| `envloader-bufio-scanner.md` | 模板示范：环境变量加载的三段式源码拆解（可复制给其他代码） |
| `interview-envloader.md` | 面试整理：口述版 + 追问复习卡 |
| `s-level-handwriting-guide.md` | S 级手写指南：三块 S 级的设计说明与路线 B 删除重写任务 |
| `README.md` | 本说明 |

## 模板怎么用

每个高价值代码块（S 级）按「三段式」沉淀：

1. **通俗课堂讲解** —— 用比喻 + 逐段走读源码，讲清为什么这么写。
2. **面试口述精炼版** —— 30~60 秒能背出来的一段话。
3. **坑点追问** —— 面试官常见变式 Q&A。

再补两块：缺陷总结、极简对比版。

## 本阶段高价值清单（对应 `docs/stages/stage-01-skeleton.md` 的 S 级）

- [x] `pkg/envloader`：`.env` 读取与「已有环境变量优先」的优先级规则（本目录已沉淀）
- [ ] `internal/deps`：依赖检查失败聚合 + 错误净化（不泄露 DSN/密码）
- [ ] `cmd/pilot`：优雅关闭（signal + context + Shutdown）
- [ ] `internal/controller`：ready 聚合与「错误 → HTTP 状态」映射

> 完成一个勾一个；面试前把每篇口述版各背一遍即可。
