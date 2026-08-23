# scx-rg 下一步开发规划

> 基于 2026-08-18 框架版本（初始提交）的现状评估。目标：把「能跑的框架」打磨成「日常可用的主力工具」。

## 定位决策

`Provider` 接口天然支持两条演进路线：

- **a. 搜索浏览器**（垂直做深）：rg 结果的实时浏览/预览/定位工具
- **b. 通用 fuzzy finder**（横向扩展）：fzf 替代品，stdin/git/docker 等多源候选 + 预览

**建议：先 a 后 b。** M1–M3 把搜索与预览体验做扎实，M4 再开放 Provider 扩展点。

## M1 搜索质量与性能（✅ 2026-08-18 完成）

| 事项 | 状态 |
| --- | --- |
| 文件模式模糊匹配 | ✅ 模糊评分（边界 +8 / 连续 +8 / 未命中惩罚），空格分词 AND，命中字符按位置高亮 |
| 搜索取消 | ✅ model 持有 cancelSearch，新搜索/封顶/退出立即杀 rg；流式生产者靠 ctx.Done 解除发送阻塞（有泄漏回归测试） |
| 内容模式流式渲染 | ✅ `SearchStream` + channel，`waitForResult` 消息链逐条送达，首条到达即跟随预览 |
| walk 模式 ignore 规则 | ✅ `rg --files` 枚举（git 仓库内尊重 .gitignore；非 git 目录回退内置遍历） |

## M2 预览增强（✅ 2026-08-22 完成）

| 事项 | 状态 |
| --- | --- |
| 预览缓存 | ✅ LRU（容量 32），key 覆盖 `path/cols/rows/jump/proto/query/size/mtime`；回访同步应用，文件增长/resize 自然失效 |
| 预览内命中词高亮 | ✅ 内容模式把搜索词传入 preview，ANSI 感知忽略大小写高亮（青色+下划线，命中后重开原语法色） |
| 多行 token ANSI 泄漏 | ✅ chroma 按 token 边界拆行、逐行独立 Format，每行自带 SGR，续行不再丢色/串色 |
| 截断↔折行 | ⛔ 决策：保持默认折行不变，不引入 `--wrap` 类 CLI 参数（项目原则：尽量不加 CLI 参数，交互能力做进 TUI）；运行时切换如需做按键，归 M4 |
| CJK 宽度验证 | ✅ 全角字符/全角字母/emoji 折行宽度 + 中文路径均有测试；reflow 与 lipgloss 计宽口径一致，无需换库 |

## M3 图片预览完善（✅ 2026-08-22 代码完成；真机视觉验证见 README checklist）

| 事项 | 状态 |
| --- | --- |
| 真机实测 | 🔶 代码侧防残留已内建并有测试：换图前删旧 placement、切走/清空注入删除序列、退出时清空全部 kitty 图形、图片预览禁滚动；kitty / ghostty / wezterm / sixel / halfblock 的视觉验证 checklist 收录在 README「真机实测 checklist（M3）」，待人工逐项确认 |
| halfblock 降级 | ✅ 第三档渲染：`▀` 半块字符 + 前景/背景双色，每字符格承载上下两像素行；truecolor → 256 色 → 16 色自动降级（termenv），无色彩输出回退占位盒；同色像素 SGR 压缩。`auto` 探测不出 kitty/sixel 时默认走 halfblock，`--img halfblock` 可显式指定 |
| DA1 精确探测 | ✅ `Detect()` 探测链：环境变量 kitty 系 → DA1 查询（`ESC[c`，raw mode + 150ms 超时读，属性 4 = sixel）→ TERM 启发式兜底 → halfblock；无 /dev/tty（管道/CI）零副作用跳过 |
| GIF 首帧 | ✅ image.Decode 解首帧（有测试：两帧 GIF 还原 kitty payload 验证像素为首帧）；动画播放维持 backlog |

附带决策与已知限制（详见 README）：
- `▀` 是歧义宽字符，lipgloss/x/ansi 按 1 格计宽（有测试锁死该假设）；「歧义宽=2」终端（iTerm2 ambiguous double）中 halfblock 会错位，可 `--img none` 规避
- SSH 场景环境变量不透传，本地 kitty 会被探测为 halfblock，可 `--img kitty` 强制
- kitty overlay 与文本滚动的完全同步需独立渲染层，维持 backlog

## M4 交互与扩展（✅ 2026-08-23 完成）

| 事项 | 状态 |
| --- | --- |
| `?` 帮助浮层 | ✅ 输入为空时按 `?` 或 `F1` 打开，任意键关闭；键位表按模式（文件/内容/finder/日志）裁剪；帧高不变式有测试 |
| 多选 | ✅ `Ctrl+Space` 标记/取消并下移（bubbletea 识别为 ctrl+@）；`Enter` 按列表顺序输出全部标记项（多行）；`Esc` 递进（输入→标记→退出）；标记 key 用 path:line 防筛选错位；`✓` 前缀 + 状态栏计数 |
| stdin Provider | ✅ `--provider stdin` 读管道候选行（非管道 stdin 报用法；上限 10 万行），复用 files 模式整套模糊语义（matchCandidates 抽取共用）；候选是真实文件路径时自动获得正常预览，否则显示详情面板；Tab/fallback 禁用 |
| 预设 Provider | ✅ `--provider docker-ps`（复用 logs.ListSources，镜像·状态做详情）；git 分支/shell 历史未内置——经 stdin 管道达成（见 examples 的 CTRL-R） |
| 配置文件 | ✅ `~/.config/scx-rg/config.toml`（BurntSushi/toml）：`debounce_ms` / `ignore`（rg `-g` + walk 双路径生效）/ `[theme]` accent·match·row_marker 三色（ApplyTheme 重设样式）；优先级 flag > config > 默认；坏文件回退默认不阻断 |
| shell 集成 | ✅ `examples/scx-rg.zsh`（zsh -n 校验）与 `examples/scx-rg.fish`：CTRL-T 文件插入、CTRL-R 历史搜索 |

附带修复/决策：
- `Result.Text` 现由文件/候选枚举统一填充——顺带修正了 finder 模式 Ctrl+Y 复制为空的问题
- 主题样式收敛到 `initStyles()` 单一定义点，ApplyTheme 换色后全局重建
- config.toml 的 ignore 同时传给 `rg -g '!dir/'` 与内置遍历

## M5 工程质量（✅ 2026-08-23 完成）

| 事项 | 状态 |
| --- | --- |
| 单测 search | ✅ rg --json 解析抽取为纯函数 `parseRgLine` 单测（match 单行/多行、begin/context/end 忽略、`./` 剥离、坏 JSON——CI 无 rg 恒跑，不再全部 skip）；walk 跳过规则含 skipDirs 与隐藏目录独立用例 |
| 单测 preview | ✅ 高亮行数对齐、CJK/emoji 折行、二进制嗅探、大文件窗口化在 M2/M3 已覆盖；本轮补 maxWrapSegments 超长单行封顶测试，并**修复截断标记把行顶超面板宽的缺陷**（末段先腾出 `...` 宽度再追加） |
| 单测 tui | ✅ drain 同步驱动 cmd 链模拟事件循环（M1 起持续使用），覆盖流式/过期/重置/多选/帮助/finder/图形清理注入 |
| golden frame 测试 | ✅ `internal/tui/golden_test.go`：五个确定性场景（files 过滤、finder 详情、帮助浮层、多选标记、图片占位）整帧去 ANSI 后与 `testdata/golden/*.txt` 对比；`go test ./internal/tui -update` 刷新基线；失败输出首差异 ±3 行。**顺手修复两个真 bug**：状态栏右侧提示超宽折行破坏帧高不变式（90 列终端触发）；帮助浮层双列在窄终端右列被整列截掉（改自适应单/双列） |
| CI | ✅ build + test + vet（ubuntu/macos 矩阵）+ `--once` 冒烟 + goreleaser/git-cliff 配置检查在 M1–M4 期间已就绪；本轮补 gofmt 检查步骤 |

## 建议的第一周切入点（按序）

1. M1 的搜索取消 + 文件模式模糊匹配（体感提升最大的两件事）
2. M3 的真终端图片实测（最早暴露架构级风险，越晚发现返工越大）
3. M5 的 golden frame 快照测试（给后续所有改动兜底）

## 风险与待验证

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| viewport 嵌图形序列在真终端的行为 | 图片功能可能需独立渲染层 | M3 已内建防残留机制（删除序列 + 禁滚动）+ halfblock 备选已落地；真机视觉验证待办（README checklist） |
| 大文件高亮卡 UI（当前在 goroutine，但 1MB 上限仍慢） | 切选卡顿 | 预览缓存 + 上限调优 + 大文件只渲染可视区 |
| bubbletea v2 正式发布 | API 迁移成本 | 锁 v1，升级列入 backlog |
