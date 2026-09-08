# 流式转换的三层心智模型

> 理解本篇是理解 Pilot 流式链路的前提。核心不是「SSE 是协议、Stream 是概念」这个结论，而是**三次流、两次转换、每层只有一个转换点**。

## 一句话总结

`STream` 这个词被三处复用，含义完全不同。真正的工作是把「供应商的裸令牌流」通过两次转换变成「HTTP 上客户能逐帧读到的字节」。

```text
[层1] 供应商/Eino 流            [层2] 业务层流                 [层3] SSE 传输
ai.TokenStream        转换A       chat.EventStream        转换B       HTTP 字节流
Recv() -> ModelChunk    ──>      Recv() -> StreamEvent       ──>      data:{...}\n\n + Flush
```

## 三次流的类型与实例

### 层1：`ai.TokenStream` —— 供应商的裸令牌流

- **接口**：`internal/ai/contract.go` 的 `TokenStream`（`Recv() (ModelChunk, error)`、`Close() error`）。
- **语义**：只说「文本增量 + usage + finish_reason」。**无意义向**——只有增量，不保证完整，不关心落库、消息 ID、取消。
- **共享一个 `ModelChunk`**（`Delta`/`Usage`/`FinishReason`），结束由返回 `io.EOF` 表达，不额外伪造结束 chunk。
- **实例（实现者）**：
  - `internal/ai/mock.go` 的 `mockTokenStream`：用 `strings.Fields` 把一句话切成词，每词一个 chunk。
  - `internal/ai/eino/openai.go` 的 `tokenStream`：把 Eino `schema.StreamReader[*schema.Message]` 的 `Recv/Close` 适配成本接口。这里完成「Eino 类型 → 项目类型」的隐藏。

### 层2：`chat.EventStream` —— 你定义的业务流（语义归一化的那一层）

- **接口**：`internal/chat/contract.go` 的 `EventStream`（`Recv() (StreamEvent, error)`、`Close() error`）。
- **语义**：只有 `content` / `done` / `error` 三种事件（`StreamEventType`）。它把层1的裸增量**累积成完整语义**，并承担全部生命周期责任——EOF 才存 assistant、context 取消要关底层流、供应商错误映射成用户可读的 `StreamError`。
- **共享一个 `StreamEvent`**（`Type`/`Content`/`Usage`/`Error`）。
- **实例（实现者）**：`internal/chat/stream.go` 的 `serviceStream`。注意——它同时持有 `upstream ai.TokenStream`（层1）和 `memory memoryStore`，职责就是**把层1转成层2**。

### 层3：SSE 传输 —— 纯 HTTP 字节流

- **没有 Go 接口**。它就是 HTTP 响应头 `Content-Type: text/event-stream` + 每帧 `data: {...}\n\n` + `http.Flusher.Flush()`。
- **语义**：完全不关心流里是 token 还是业务事件，只做协议适配。
- **实例（实现者）**：
  - `internal/handler/sse.go`：`setSSEHeaders`（设头）+ `writeSSEEvent`（把 `StreamEvent` JSON 序列化并写上 `data: ...\n\n`）。
  - `internal/handler/chat_stream.go` 的 `ChatHandler.Stream`：循环 `EventStream.Recv()` → `writeSSEEvent` → `Flush()`。

## 两次转换的边界（每层只做一件事）

### 转换A：层1 → 层2（在 `internal/chat`，做语义归一）

`serviceStream.Recv()` 每次调用的决策链：

| 层1 返回 | 层2 产出 | 生命周期动作 |
| --- | --- | --- |
| `(chunk, nil)`, Delta 非空 | `content` 事件 | 累积到 `strings.Builder` |
| `(chunk, nil)`, Delta 为空 | 跳过,继续读 | 不发空事件 |
| `(chunk, nil)`, Usage 非空 | （合并进最终 `done`） | 记录 usage |
| `(_, io.EOF)`, context 有效 | `finish()` → 先存完整 assistant → `done` | 关闭底层流,`finished=true` |
| `(_, io.EOF)`, context 已取消 | 返回 `context.Canceled` | 不存 assistant |
| `(_, err)` context 已取消 | 返回 `context.Canceled` | 不存 assistant |
| `(_, err)` 其它 | `error` 事件（`UPSTREAM_ERROR`） | 不存 assistant |

- 这里也解释了「`done` 之后下一次 `Recv` 才是 `io.EOF`」：`finish()` 把 `finished` 置为 `true`，下次 `Recv` 直接返回 `io.EOF`。`done` 告诉客户端「最后一条」，`io.EOF` 告诉适配器「流已断开」。
- `Close()` 幂等：`defer stream.Close()`（层3）和 `serviceStream` 内部（`finish`/错误分支）都会调用，靠 `closed` 标志保证底层 `TokenStream.Close()` 只真正执行一次。

### 转换B：层2 → 层3（在 `internal/handler`，做协议适配）

`Chat.Stream` 的循环：

| 层2 返回 | 层3 动作 |
| --- | --- |
| `(event, nil)` | `writeSSEEvent(w, event)` → `Flush()` |
| `({}, io.EOF)` | `return`，结束连接 |
| `({}, 其它 error)` | `return`，静默断开 |

- **一个关键设计**：层3 对「非 `io.EOF` 的 error」选择静默断开，而不是再补发一个 `error` 帧。因为错误的语义归一（`error` 事件）已经发生在转换A——层2 就该把错误变成 `StreamEvent{Type: error}` 返回出来。层3 再收到裸 error，只意味着连接断了，不该重复发帧。

## 分级与手写边界

- **A 级**：`EventStream`/`TokenStream`/`StreamEvent`/`ModelChunk` 契约定义，`writeSSEEvent`/`setSSEHeaders` 这类纯序列化，`Chat.Stream` 的请求解析与 `defer Close`。
- **S 级（用 `BEGIN/END S_LEVEL_REFERENCE` 标注，需独立手写）**：
  - `serviceStream.Recv()` 的完整决策链（`chat/stream.go`）。
  - `serviceStream.finish()` 的 EOF 持久化与空响应判定。
  - `Chat.Stream` 的 SSE 事件循环（`controller/chat_stream.go`）。

## 一句话串起来

「供应商 SDK 返回的 token」（层1）被 `serviceStream` 吃掉、累积、判定生死，产出「业务语义流」（层2）；`Chat.Stream` 再把这个语义流序列化成「HTTP 上的 SSE 帧」（层3）。两层转换各自独立，互不泄漏对方的世界——这就是为什么 `controller` 从不 import 供应商 SDK，`serviceStream` 从不碰 `http.Flusher`。
