# 关键设计决策存档

这些决策大多以代码注释形式锚在实现处，本页汇总「为什么」——改动相关区域前先读对应条目。

## 1. ASCII 边框 + 歧义宽字符保守计宽

**现象**：中文环境终端常把 East Asian Ambiguous 字符（Unicode 制表符、·…× 等）按 2 格渲染，而 lipgloss/x/ansi 按 1 格计宽——帧里出现这类字符时行宽在歧义宽终端超出终端宽，触发软换行，bubbletea diff 渲染器行号错位（表现为上下切换后输入框/标题重复残留的「鬼影」）。

**决策**：
- 界面边框/分隔符全部 ASCII（`+-|`，styles.go、code.go 行号槽）
- preview 包 init 设 `runewidth.EastAsianWidth = true + CreateLUT()`：内容折行/截断按歧义=2 格**保守**计算（普通终端提前折无害）
- 回归测试 `frame_width_test.go`：帧行数=终端高度、`lipgloss.Width + 歧义字符数 ≤ frameW` 不变式

**注意**：改宽度条件必须重建 LUT（包 init 编译期），否则不生效。halfblock 的 `▀` 也是歧义宽——按 lipgloss=1 格口径渲染（有测试锁死该假设），歧义宽=2 终端中 halfblock 会错位（已知限制，`--img none` 规避）。

## 2. frameW() = 终端宽 - 1

帧行恰好占满终端宽时行尾压在 wrap-pending 边界，部分终端引擎（Termius 本地终端）高频局部重绘对该边界处理有缺陷→鬼影。收窄 1 列后行尾自带「清除行尾」序列，重绘自清洁（model.go `frameW` 注释）。

## 3. 状态栏恒一行（statusLine helper）

状态栏右侧按键提示在窄终端（90 列）超宽，lipgloss `Width()` 的 wordwrap 会把帧撑高、破坏帧高不变式——golden frame 落地时抓出的 bug。决策：`statusLine(left, right)` 左侧超宽截左、右侧超宽截右，帧高恒定优先于提示完整。

## 4. kitty overlay 图形清理链

kitty 图形协议是 overlay（不占字符流），文本替换/清屏不会清掉它。四层清理：每次输出前删旧 placement（幂等前缀）→ 切到非图形内容注入删除序列（setPreviewContent）→ 预览清空借空态提示帧送删除（view.go，一次性消费标志）→ 程序退出写 stdout 全删。sixel 是内联图形随文本走，无需对应机制。图片预览同时禁用 PgUp/PgDn（overlay 不随文本滚动）。

## 5. DA1 探测：非阻塞轮询而非 SetReadDeadline

macOS 的 `/dev/tty` 对 `SetReadDeadline` 返回 "file type does not support deadline"，裸 `Read` 在终端不响应时**永久阻塞**（曾导致启动卡死）。决策：`unix.SetNonblock(fd)` + 10ms 间隔轮询、150ms 总预算，无 goroutine 泄漏；失败零副作用回退环境变量启发式。探测链：env kitty 系 → DA1(sixel 属性 4) → TERM 启发式 → halfblock。

## 6. 预览缓存 key 覆盖全部渲染输入

`{path, cols, rows, jump, proto, query, size, mtime}`（cache.go）——size/mtime 让 --follow 文件变化自然失效、proto 进 key 防协议切换串档、cols/rows 让 resize 自然 miss。「回访同步应用」依赖 key 完备性，加渲染输入字段时必须同步改 key。

## 7. 尽量不加 CLI 参数

项目原则（PLAN M2 决策，--wrap 被拒绝的案例）：交互能力做进 TUI（键位/配置文件），CLI 面保持克制。新增参数需先质疑：能否做成按键（如 Ctrl+F 切换）或 config.toml？

## 8. 搜索取消不泄漏

新搜索/封顶/退出 → `cancelSearch()` 立即杀 rg 进程；流式生产者 `select { ch<-res; <-ctx.Done() }`——消费者停读时靠取消解除发送阻塞。有专门回归测试防 goroutine 泄漏。

## 9. rg 退出码语义与容错

0=有匹配、1=无匹配（**不是错误**）、2+=错误。文件枚举的 exit 2 容错（stdout 有结果就可用）针对 macOS TCC 隐私目录；内容搜索仅在「非零非 1 且零结果」时把 stderr 提炼成一条错误经结果流传回。

## 10. 跨行 token 的逐行 Format

chroma 整文 format 再按 \n 切行会让跨行 token（块注释、多行字符串）的颜色泄漏到相邻行或经 reset 丢色。`highlightLines` 按 token 边界拆行、每行独立 Format 自带 SGR（code.go 注释，M2 修复）。

Related: [preview](../02-modules/preview.md) · [tui](../02-modules/tui.md) · [testing（帧不变式测试）](../03-guides/testing.md) · [docs/PLAN.md（原始决策记录）](../../docs/PLAN.md)
