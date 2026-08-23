# preview —— 预览渲染

包路径：`internal/preview`。职责：把文件变成可直接进入 viewport 的 ANSI 内容（代码高亮 / 图片编码），带 LRU 缓存。**唯一入口 `Render(path, cols, rows, proto, jump, query)`**（tui 的 renderFile 字段默认指向它，测试可注入 fake）。

## 分发（preview.go）

- 扩展名命中 `imageExts`（png/jpg/jpeg/gif/webp/bmp/tif/tiff）→ `renderImage`
- 其余 → `renderCode`
- 包 init 设置 `runewidth.EastAsianWidth = true + CreateLUT()`：歧义宽字符按 2 格保守计宽（见 [design-decisions](../04-decisions/design-decisions.md)）

## 代码渲染（code.go）

| 常量 | 值 | 作用 |
| --- | --- | --- |
| `maxCodeBytes` | 1MB | 全量渲染上限，超出走窗口化 |
| `maxCodeLines` | 3000 | 行数上限 |
| `windowBefore/After` | 40 / 80 | 窗口化时 jump 行前后上下文 |
| `maxWrapSegments` | 10 | 单源行最多折 10 段，超出 `...` 收尾（防压缩 JSON 撑爆视口；末段先腾出省略号宽度，不超面板宽） |

关键机制：

- **窗口化渲染**（>1MB）：只渲 jump 前后窗口 + 「前面省略」标记；此时真实行号≠物理行号，`Rendered.JumpOffset` 记录 jump 的物理行号，tui 滚动定位必须用它
- **跨行 token 颜色**：`highlightLines` 按 token 边界拆行、逐行独立 Format，每行自带 SGR——整文 format 再切行会让跨行 token（块注释/多行字符串）串色
- **命中词高亮**：内容模式 query 经 `highlightTermANSI` 在已含 ANSI 的行内做忽略大小写高亮（青色+下划线，重开原语法色）
- **色彩档位**：`formatterName()` 按 `termenv.ColorProfile()` 选 chroma formatter（terminal16m/256/tty）；`formatterFor` 变量可被测试覆盖
- **二进制嗅探**：NUL 字节 → `KindBinary`
- 折行 ANSI-aware（reflow/wrap），宽度按 EastAsianWidth 口径，CJK/emoji 有测试

## 图片渲染三档（image.go / halfblock.go / protocol.go）

协议探测链（`Detect()`，main 启动时一次）：

```text
1. 环境变量 kitty 系：KITTY_WINDOW_ID / TERM 含 kitty|ghostty / GHOSTTY_RESOURCES_DIR /
   WEZTERM_PANE|EXECUTABLE → kitty（kitty 协议无 DA1 标志，环境变量是唯一可靠依据）
2. DA1 查询：ESC[c → raw mode + 非阻塞轮询 150ms（da1_unix.go；/dev/tty 不支持
   SetReadDeadline，裸 Read 会永久阻塞——踩过的坑）；响应属性 4 = sixel
3. TERM 启发式兜底：foot / yaft / mlterm → sixel
4. halfblock（任何彩色终端）
```

| 档 | 实现 | 要点 |
| --- | --- | --- |
| kitty | `kittyBlock`：整图 PNG base64 按 4096 分块，`a=T,i=7`（固定 id 复用）；输出前缀删除序列；末尾补 r-1 空行对齐 viewport 行数 | overlay 不随文本消失→清理链见 [tui](tui.md) |
| sixel | `sixelBlock`：`scaleFit` 缩到面板像素框再编码（mattn/go-sixel） | 内联图形，随文本滚动 |
| halfblock | `halfblockBlock`：`▀` 半块字符前景=上像素/背景=下像素，缩到 cols×2rows；termenv Profile.Convert 降级 truecolor→256→16；同色 SGR 压缩 | 纯文本无残留；无色输出（管道/CI）回退占位盒 |

`fitCells` 按 cell 像素比（`cellSize()` = TIOCGWINSZ 像素/行列，失败兜底 10x20）算等比占位。GIF 解首帧（image.Decode，有测试锁定）。

## 渲染缓存（cache.go）

`Cache` LRU 容量 32（tui 持有），key 为渲染输入**全依赖**：`{path, cols, rows, jump, proto, query, size, mtime}`——size/mtime 让 --follow 等文件变化自然失效；proto 进 key 使协议切换不串档。Get 在 Update 循环、Put 在渲染 goroutine，mutex 并发安全。

Related: [tui（调用方与图形清理链）](tui.md) · [design-decisions（宽度口径）](../04-decisions/design-decisions.md) · [README 图片协议章节](../../README.md)
