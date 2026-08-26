# 未完成清单（TODO）

> 本清单记录 Stage 01 尚未收尾的事项；完成一项勾一项，作为下一阶段开始前的门槛。

## S 级手写（路线 B：删除参考实现重写）

- [ ] S1 优雅关闭（`zqk_hand_write/mock_main.go`）：`case err := <-errCh` 方向、`main/run` 结构、`cfg/srv` 装配、收到信号后的 `WithTimeout + srv.Shutdown` 收尾
- [ ] S2 依赖检查聚合 + 错误净化（`internal/deps`）：尚无手写稿
- [ ] S3 ready 聚合与状态映射（`zqk_hand_write/health.go`）：状态码传参硬伤、`toCheckViews`/`readyResponse`/`LatencyMS` 命名、`Live` 空实现
- [ ] env_loader 手写（A 级巩固，可选）：`strings.Cut` + `LookupEnv` + `Setenv` 三段落笔

## 代码完善

- [ ] `internal/deps` 补 `CheckAll` 聚合测试（一成一败）
- [ ] `configutil.Validate()`：非法配置显式报错，替代静默回退默认值
- [ ] `ALLOW_DEGRADED` 语义落地（观测依赖降级，Stage 06）

## 验收与交付

- [ ] 手写版替换进仓库后跑通 `go test ./...`
- [ ] 创建 `stage/01-skeleton` 分支（当前 detached HEAD）
- [ ] 口述验收：能讲清数据流、分层职责、方案取舍
- [ ] `stages/THOUGHT.md` 复盘题回答 + 面试口述素材填写
- [ ] Docker Compose 集成跑通（先处理本机 3306 与 MySQL 冲突）

## Stage 02 收尾

- [x] `memory.Service.LoadRecent`：缓存命中、MySQL 回源和 Redis 回填
- [x] `memory.Service.SaveMessages`：MySQL 优先写入和 Redis 失败策略
- [x] fake store 单元测试和 `go test ./...`
- [ ] Compose 环境下验证真实 MySQL/Redis 读写
- [ ] 补充同一会话并发写入顺序策略
- [ ] 更新 Stage 02 学习者回答和面试口述素材
