# 面试整理：环境变量加载（复习卡）

> 对应 `pkg/envloader/envloader.go`。背熟「口述版」，扫一遍「追问速答」，考前 5 分钟过「记忆点」。

## 30 秒口述版

"`os.File` 只是文件句柄，只有原始字节读取，没有按行分割能力。`bufio.Scanner` 封装了缓冲 IO，自动识别换行、按行迭代，不把整个文件一次性载入内存，适合流式读配置文件；循环结束必须调用 `scanner.Err()` 区分正常 EOF 与 IO 错误。代价是默认单行 64KB 上限，超长行会报 `bufio.ErrTooLong`。业务上还做了：文件不存在静默忽略、已有环境变量优先不覆盖、用 `strings.Cut` 切第一个等号（value 可含等号）、用 `LookupEnv` 判断变量是否真实存在。"

## 追问速答卡

- **为什么不用 `ReadFile`？** 一次性全文进内存，大文件内存压力大；它没有区分"读取出错"的便捷逻辑。
- **`ReadFile` 什么时候够用？** `.env` 很小、几百行以内；写起来更短。
- **`scanner.Scan()` 返回 false 意味着什么？** 读完 EOF，或读取出错——必须靠 `scanner.Err()` 区分。
- **漏写 `scanner.Err()` 的后果？** 磁盘损坏、读一半断流会被当成正常结束，静默吞掉 IO 异常。
- **单行超过 64KB？** `Scan()` 返回 false，`Err()` 返回 `bufio.ErrTooLong`；用 `scanner.Buffer` 扩容量。
- **`defer f.Close()` 放哪？** 放在 `Open` 成功之后、`err` 判断之后；Open 失败时 f 为 nil，直接返回，不会走到 defer。
- **`strings.Cut` 找不到 `=`？** 返回原串、`ok=false`，代码捕获后报 `missing '='`。
- **value 里含 `=`？** `Cut` 只在第一个 `=` 切一次，`API_KEY=abc=123` → key=`API_KEY`、value=`abc=123`。
- **`LookupEnv` vs `GetEnv`？** `GetEnv` 区分不了"不存在"和"空串"；`LookupEnv` 返回 bool 判断是否真实存在。
- **`os.Setenv` 并发安全吗？** 不安全，多 goroutine 会 data race；只应在初始化阶段调用。
- **Windows `\r\n`？** `bufio.ScanLines` 自动处理，另加 `TrimSpace` 兜底。
- **本实现缺陷？** 64KB 上限、不支持引号、不支持转义、Setenv 非并发安全。

## 记忆点（背这个列表）

- `*os.File`：无按行能力，只有 `Read([]byte)`。
- `bufio.Scanner`：流式逐行 + 自动处理换行。
- 必须 `scanner.Err()`：区分 EOF 与 IO 错误。
- 默认单行 64KB，超长报 `bufio.ErrTooLong`。
- `strings.Cut`：切第一个 `=`，支持 value 含 `=`。
- `LookupEnv`：判是否存在（vs `GetEnv` 判内容）。
- 已存在变量优先，不被 `.env` 覆盖（配置优先级：真实环境变量 > `.env`）。
- `os.Setenv`：进程内生效、非并发安全、仅初始化调用。
