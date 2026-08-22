# 源码拆解模板：Go .env 加载为什么要用 bufio.Scanner？

> 三段式训练模板：通俗课堂讲解 → 面试口述版 → 坑点追问，Abdul Bari 讲课风格。
> 示范对象：`pkg/envloader/envloader.go`。后续分析其他高价值代码时，复制本文件骨架。

## 0. 分析对象

- 文件：`pkg/envloader/envloader.go`
- 功能：从 `.env` 读取 `KEY=VALUE` 写入进程环境变量，**已存在的环境变量不被覆盖**。
- 为什么是高价值：体现「配置单一来源 + 优先级」的心智模型，是常见的面试切入面。

先看两种思路对比：

1. 简单粗暴：`ioutil.ReadFile` 一次性把整个 `.env` 全部读进内存，再按 `\n` split 切行。
2. 代码里的做法：`bufio.NewScanner(f)` 按行迭代读取，一行一行处理。

## 1. 通俗课堂讲解

想象你要读一本很厚的书（`.env` 文件）。

- `ReadFile`：直接把整本书复印一整本放到你桌上。文件超大，桌上堆得满满当当，内存暴涨。小文件无所谓；几千行上万行时内存开销变大。
- `bufio.Scanner`：**一次只翻一页（一行）**，读完一页处理完就扔掉，内存只存当前这一行。

`os.Open(path)` 返回 `*os.File`，它是**流式句柄**，底层是系统文件描述符，本身**没有按行读取能力**。`*os.File` 只有 `Read([]byte)`：给一块缓冲区，读一堆字节，它不懂什么叫"换行符"。你拿到字节后还要自己找 `\n`、切行、处理不完整行，还要处理 `\r\n` Windows 换行，非常麻烦。

👉 **`bufio.Scanner` 封装好了"按行切分"**：内部自带缓冲区，自动识别 `\n` / `\r\n`，自动处理缓冲区边界，循环 `scanner.Scan()` 每次给到你完整一行字符串。

逐段走读函数逻辑：

```go
f, err := os.Open(path)
if err != nil {
    if os.IsNotExist(err) {
        return nil // 文件不存在直接静默返回 nil，符合注释需求
    }
    return fmt.Errorf("open env file %s: %w", path, err)
}
defer f.Close() // 一定要关闭文件句柄，防止 fd 泄漏
```

> 重点：`os.Open` 只读打开，不会创建文件。文件不存在不是报错，直接 `return nil`，这是 `.env` 库常见行为。

```go
scanner := bufio.NewScanner(f)
line := 0
for scanner.Scan() { // true：读到一行；false：读到文件末尾 / 发生 IO 错误
    line++
    text := strings.TrimSpace(scanner.Text()) // TrimSpace 去掉首尾空白（含 \r）
    if text == "" || strings.HasPrefix(text, "#") {
        continue // 空行、# 注释跳过
    }
```

`scanner.Scan()` 返回 `false` 时**区分两种情况**：
1. 正常读完文件结束；
2. **读文件时发生 IO 错误**。

> ⚠️ 非常多新手踩坑：循环结束后，必须调用 `scanner.Err()`。

```go
if err := scanner.Err(); err != nil {
    return fmt.Errorf("read env file %s: %w", path, err)
}
```

如果不写这句：磁盘损坏、读一半断流，`scanner.Scan()` 返回 false 退出循环，你会以为文件正常读完，忽略 IO 异常——这是 Scanner 最大的坑。

接下来解析 `KEY=VALUE`：

```go
key, value, ok := strings.Cut(text, "=")
if !ok {
    return fmt.Errorf("%s:%d: missing '='", path, line)
}
key = strings.TrimSpace(key)
value = strings.TrimSpace(value)
if key == "" {
    return fmt.Errorf("%s:%d: empty key", path, line)
}
```

`strings.Cut`（Go 1.20+）按**第一个 `=`** 切成两部分。举例子：`API_KEY=abc=123` → `key=API_KEY`、`value=abc=123`，符合 env 文件语法。若用 `strings.SplitN(text, "=", 2)` 效果等价。

业务规则：**已经存在的环境变量，不被 `.env` 覆盖**。

```go
if _, exists := os.LookupEnv(key); exists {
    continue
}
if err := os.Setenv(key, value); err != nil {
    return fmt.Errorf("%s:%d: set %s: %w", path, line, key, err)
}
```

`os.LookupEnv` 区别于 `os.Getenv`：`Getenv` 无法区分"变量不存在"和"变量为空字符串"；`LookupEnv` 返回 bool 标识是否真实存在，所以用它来判断"是否已存在"。

### 那什么时候直接用 ReadFile？

- `.env` 确定很小（几百行以内）时，一次性读进来 `bytes.Split(content, '\n')` 更短。
- 缺点：大文件一次性全量进内存；没有内置"区分读取错误"的便捷逻辑。

### bufio.Scanner 的隐藏限制！

Scanner **默认最大单行缓冲区 64KB**。`.env` 某一行超过 64K（比如超长密钥），`scanner.Scan()` 返回 false，`scanner.Err()` 返回 `bufio.ErrTooLong`。

> 本实现没有处理超长行，是一个缺陷。如需支持，要手动扩大 scanner 的 buffer。

## 2. 面试口述精炼版（背这段）

> 面试官：为什么用 `bufio.Scanner` 读 env 文件，不一次性 `ReadFile`？

"首先 `os.File` 本身只是文件句柄，只支持原始字节读取，没有按行分割能力。`bufio.Scanner` 封装了缓冲 IO，自动识别换行符、按行迭代，不需要把整个文件一次性加载进内存，适合流式逐行解析配置文件。循环结束**必须调用 `scanner.Err()`**，区分正常 EOF 和 IO 读取错误，很多人漏掉这一步，会漏掉磁盘读写异常。

对比 `ReadFile`：小文件没问题，但大文件会一次性占用全部文件内存；另外 Scanner 有默认 64KB 单行上限，配置存在超长单行会报错，这点要注意。

这个实现还有业务逻辑：文件不存在时静默忽略；已有环境变量优先、不覆盖；解析时处理注释和空行；用 `strings.Cut` 分割第一个等号，支持 value 内再含等号；用 `LookupEnv` 判断环境变量是否真实存在，而不是 `GetEnv`。"

## 3. 面试官变式追问

### Q1：如果 `.env` 一行超过 64KB，会发生什么？怎么修复？

A：`scanner.Scan()` 返回 false 退出循环；`scanner.Err()` 返回 `bufio.ErrTooLong`，函数返回错误。
修复：用 `scanner.Buffer()` 扩大内部缓冲区：

```go
const maxScanTokenSize = 1024 * 1024 // 1MB
buf := make([]byte, maxScanTokenSize)
scanner.Buffer(buf, maxScanTokenSize)
```

### Q2：`defer f.Close()` 写在 `if err != nil` 之后，有没有问题？

```go
f, err := os.Open(path)
if err != nil {
    return ...
}
defer f.Close()
```

✅ 没问题。Open 失败时已 `return`，`f` 为 nil，不会注册 defer；Open 成功才注册关闭。
> 错误写法：先写 defer 再判断 err，Open 失败时 `f` 是 nil，`Close()` 调用 nil 指针会 panic。

### Q3：为什么不用 `ioutil.ReadAll(f)`？

A：`ReadAll` 也是全部读到内存，和 `ReadFile` 一样全量加载，大文件内存压力相同。

### Q4：`os.Setenv` 是进程全局生效吗？协程安全吗？

A：`os.Setenv` 修改的是 Go 进程内部的环境变量表，只对当前进程生效。**Go 里 `Setenv` 不是协程安全的**，多 goroutine 同时调用会发生 data race。所以 `Load` 适合在程序初始化阶段单线程调用，不要放业务运行期多协程并发调用。

### Q5：`strings.Cut` 找不到分隔符时返回什么？

A：返回原始字符串，`ok=false`。代码正好捕获该情况，报错 `missing '='`。

### Q6：Windows 换行 `\r\n`，Scanner 能处理吗？

A：✅ 能。Scanner 默认分割函数 `bufio.ScanLines` 自动处理 `\n` 和 `\r\n`，每行返回时已剔除换行符，无需手动处理 `\r`（代码里的 `TrimSpace` 又兜了一层）。

## 4. 本实现缺陷总结

1. Scanner 单行上限 64KB，超长行直接报错，没有自定义 buffer。
2. 不支持引号：`KEY="hello world"` 会把 value 读成带双引号的 `"hello world"`，未去引号。
3. 不支持转义字符 `\`。
4. `os.Setenv` 非并发安全，只能在 main 初始化阶段调用。

## 5. 极简 ReadFile 对比版（不推荐生产）

```go
func LoadSimple(path string) error {
    content, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return err
    }
    lines := strings.Split(string(content), "\n")
    for _, lineStr := range lines {
        // 解析逻辑与上面一致
    }
    return nil
}
```

缺点：整个文件全部加载进内存；`Split("\n")` 在 Windows 下残留 `\r`，要额外 Trim；无法区分读取错误与文件内容本身的问题。
