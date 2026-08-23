# search —— 搜索后端

包路径：`internal/search`。与文件系统耦合仅限枚举；打分是纯字符串函数，finder 模式直接复用。

## Provider 抽象（provider.go）

```go
Provider        // Name() 标识
SyncProvider    // Search(ctx, root, query) ([]Result, error) —— files / list 模式
StreamProvider  // SearchStream(ctx, root, query) (<-chan Result, error) —— content 模式
```

`Result{Path, Line, Text, Hits, Detail, Err}`：
- files/finder 模式 `Line=0`、`Path`=文件相对路径或候选行、`Hits`=模糊命中 rune 下标（列表高亮用）
- content 模式 `Line`=匹配行号、`Text`=行文本
- `Detail` 仅 finder（docker-ps 的镜像·状态）
- `Err` 非空表示搜索本身失败，终止消费

`MaxResults = 500` 是所有列表的硬上限。

## files 模式（files.go）

`FilesProvider{UseRg, Exact, IgnoreExtra}`：

1. **枚举**：`UseRg=true` 走 `ListFiles`——`rg --files -g '!dir/'...`（config 的 ignore 追加为 glob），**exit 2 容错**：rg 遍历遇个别不可读目录（macOS TCC 隐私目录）会 exit 2 但 stdout 已有结果，只有空输出才报错；`UseRg=false` 走 `walkFiles` 内置遍历（skipDirs 名单 + `.` 前缀隐藏 + IgnoreExtra + 排序）。
2. **打分**：`matchCandidates(candidates, query, exact)`——files 与 finder 共用（M4 抽取）：
   - `Fuzzy(query, text)`：空格分词 AND、子序列匹配、边界/连续加权、散落拼凑匹配标记 `Scattered` 并过滤（宁缺毋滥）
   - `ExactMatch`：分词必须是完整子串（Ctrl+F 切换）
   - 非空 query 按分数降序（同分按路径字典序）；空查询不排序直接截断到 MaxResults

## content 模式（rg.go）

`RipgrepProvider{Literal}`：

- 命令：`rg --json --smart-case [--fixed-strings] -- <query> .`，工作目录 = root
- **解析**：`parseRgLine(line)` 纯函数（有独立单测，CI 无 rg 也跑）——match 事件产出 `Result{Path: 去 ./ 前缀, Line, Text: TrimSpace(lines.text)}`；begin/context/end/summary/坏 JSON/空 path 一律跳过
- **退出码语义**：0=有匹配，1=无匹配（不是错误），2+=错误；仅「非零且非 1 且零结果」时把 stderr 提炼（`rgErrMessage`：去 `rg: ` 前缀 + 拼 error 行）经结果流传回
- **取消**：ctx 取消立即杀 rg 进程；发送侧 `select { ch<-res; <-ctx.Done() }` 防止消费者停读导致 goroutine 泄漏（有泄漏回归测试）
- 上游约束：tui 层对非法正则查询自动按字面量兜底重发（用户搜 `log.error(` 不报错）

## 模糊评分（fuzzy.go）

子序列匹配 + 加权：边界命中 +8（`/`、`-`、`_`、驼峰、首字符）、连续段 +8、未命中字符惩罚；完整子串命中大幅优先。分词 AND 语义：每个空格分词都要命中。测试在 fuzzy_test.go（评分/分词/散落过滤）。

## finder 模式（list.go）

`ListProvider{Candidates, Exact}`：静态候选列表（stdin 行 / docker 容器）直接喂 `matchCandidates`，零文件系统依赖——与 files 模式同一套匹配语义。

Related: [architecture](../01-overview/architecture.md) · [tui（调用方）](tui.md) · [interaction（finder 用法）](../03-guides/interaction.md)
