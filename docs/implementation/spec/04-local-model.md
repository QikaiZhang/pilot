# 本地 Mock 模型规格

没有 LLM 或 Embedding 账号时，先用 Mock 完成工程链路；替换真实模型时不得改变 Controller、Memory、Retriever 和测试接口。

## ChatModel 接口

```go
type ChatModel interface {
    Generate(ctx context.Context, messages []Message) (string, error)
    Stream(ctx context.Context, messages []Message) (<-chan string, <-chan error)
}
```

Mock `Generate` 返回固定格式：`已收到：<最后一条 user 内容>`；`Stream` 每 20ms 发送一个词，并在 context 取消时关闭两个 channel。测试必须覆盖正常结束、模型错误和客户端取消。

## Embedder 接口

```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    Dim() int
}
```

Mock Embedding 用稳定 hash 将文本映射成 `EMBEDDING_DIM` 个浮点数，再归一化为单位向量。它不代表真实语义质量，只用于验证索引、查询、RRF 和错误路径。

## 切换真实模型

只替换 `ChatModel` 和 `Embedder` 的实现及配置；先保留 Mock 合约测试，再增加供应商集成测试。真实模型超时、限流、空响应必须统一映射为 `UPSTREAM_ERROR`，不能让供应商 SDK 类型泄漏到 Controller。
