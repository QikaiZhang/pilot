# Stage 01 · S 级手写指南

> 路线 B【删除重写】：AI 提供可运行参考实现并强标记，学习者**先读设计说明 → 在参考实现上自己注释理解 → 删除 `BEGIN/END S_LEVEL_REFERENCE` 区域 → 从零独立重写 → 跑测试**。
> 对应 `docs/stages/stage-01-skeleton.md` 的 S 级边界：优雅关闭、依赖检查失败聚合、错误到 HTTP 状态映射。

---

## S1 优雅关闭（`cmd/pilot/main.go`）

### 为什么是 S 级
进程生命周期边界。处理不好会丢进行中的请求、泄漏 goroutine/连接、无法平滑发布；面试几乎必问"如何优雅下线"。

### 设计意图
收到信号 → 停止接收新连接 → 给存量请求宽限时间 → 超时强制退出。

### 手写要点
1. `signal.NotifyContext` 监听 `SIGINT/SIGTERM`，`defer stop()` 释放监听。
2. **Shutdown 必须用新的超时 ctx，不能复用被信号取消的 ctx**（信号 ctx 已 done，Shutdown 会立刻超时失效）。
3. `errCh` 容量为 1，防止 ListenAndServe 的 goroutine 阻塞泄漏。
4. `ListenAndServe` 返回 `http.ErrServerClosed` 视为正常关闭，不能当错误上报。
5. 超时后可选 `srv.Close()` 强制断开（面试可提"宽限 vs 强停"的取舍）。

### 手写任务
- 删除 `main.go` 中 `BEGIN/END S_LEVEL_REFERENCE` 区域后重写。
- 自验：`make run` 启动后在另一终端按 `Ctrl+C`，日志必须出现 `server stopped cleanly`，退出码 0。

### 面试追问
- 为什么 Shutdown 不能用信号 ctx？
- 超时之后会发生什么？线上平滑发布还缺什么（健康摘除、连接 drain、停机窗口）？

---

## S2 依赖检查失败聚合 + 错误净化（`internal/deps`）

### 为什么是 S 级
启动失败边界 + 凭据安全。三个依赖应**一次报告全貌**（聚合而非短路）；错误不能泄露 DSN/密码（写进日志或 HTTP 响应的都算泄露）。

### 设计意图
`CheckAll` 逐项检查、失败继续；`Result` 携带 `name / ok / latency / err`；同一份结果同时喂给启动失败信息和 `/ready`。

### 手写要点
1. 循环执行**所有** Dependency，不能第一个失败就 return（否则运维看不到完整故障面）。
2. 记录每次耗时：`time.Since(start)` → `LatencyMS`。
3. 每个错误**必须带依赖名前缀**（`fmt.Errorf("redis: %w", ...)`）。
4. MySQL：`sql.Open` 失败多为 DSN 非法，driver 错误可能**回显 DSN**，必须掩码为 `mysql: invalid DSN`。

### 手写任务
- 删除 `deps.go` 中 `CheckAll` 的参考实现区域重写；处理 MySQL 掩码区域。
- Redis/ES 的错误包裹（`fmt.Errorf("redis: %w")`）同属"净化"范畴，重写时自查。
- **补测试**：`internal/deps` 目前没有单测，手写时给 `CheckAll` 加聚合测试（一成一败，验证两个结果都在、失败项 `Err` 非 nil）。

### 面试追问
- 为什么聚合而不是短路？
- 启动检查和 `/ready` 复用同一套 CheckAll，怎么保证 `/ready` 不会拖死请求？（超时 ctx）

---

## S3 ready 聚合与错误 → HTTP 状态映射（`internal/controller`）

### 为什么是 S 级
对外契约。状态码语义错了会误导调用方和监控告警；错误体泄露内部细节等于给攻击者递情报。

### 设计意图
全部可用 → 200；任一失败 → 503（unavailable 语义），响应体逐项报告 `name/status/latency_ms/error`。统一错误体 `{code, message}` 是后续所有接口的格式约定。

### 手写要点
1. 用 `context.WithTimeout(r.Context(), 3s)` 限制单次检查总时长。
2. 用 `deps.AllOK` 决定 200/503，错误只透出已净化信息。
3. 统一错误体由 `WriteError` 输出，`{code: "machine_readable", message: "人可读"}`，不把内部堆栈给客户端。

### 手写任务
- 删除 `health.go` 中 `Ready` 的参考实现区域重写。
- 自验：`go test ./internal/controller/` 三个用例（live / allOK / failure）必须通过。

### 面试追问
- 503 和 500 怎么选？健康检查该不该暴露详细错误（内网可、公网收敛）？
- 如果依赖检查耗时超过 3s，你会怎么处理？（缩短超时 / 并发检查 / 缓存上次结果）

---

## 完成清单

- [ ] S1 已注释理解并重写（`cmd/pilot/main.go`）
- [ ] S2 已注释理解并重写（`internal/deps`）+ 补 CheckAll 聚合测试
- [ ] S3 已注释理解并重写（`internal/controller/health.go`）
- [ ] `go test ./...` 全绿
- [ ] 三块都能用一句话讲清"为什么这么写"
