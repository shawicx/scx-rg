# 项目总览

## 定位

scx-rg 是一个**基于 ripgrep 的终端搜索浏览器**，同时通过 `--provider` 演进为**通用 fuzzy finder**（fzf 替代品的管道模式）。单二进制 Go TUI，macOS / Linux × amd64/arm64。

两条产品线（docs/PLAN.md 的定位决策，先 a 后 b，均已落地）：

- **a. 搜索浏览器**：rg 结果的实时浏览 / 预览 / 定位（文件模式 + 内容模式 + 日志跟随）
- **b. 通用 finder**：`--provider stdin | docker-ps` 读外部候选做模糊筛选（M4）

## 核心能力

| 能力 | 入口 |
| --- | --- |
| 实时防抖搜索（200ms，版本号判废过期结果） | 主界面输入框 |
| 文件模式：模糊匹配（分词 AND、边界/连续加权、散落噪声过滤）或精确子串 | 默认模式，`Ctrl+F` 切换 |
| 内容模式：rg --json 流式、边搜边出、非法正则自动字面量兜底 | `Tab` 切换 |
| 文件名零命中自动全文回退 | 文件模式行为 |
| 多面板预览：chroma 语法高亮 / 行号槽 / 匹配行跳转 / 大文件窗口化 | 右侧面板 |
| 图片预览三档：kitty / sixel / halfblock（▀ 半块 + truecolor 降级） | 选中图片文件 |
| 多选输出（Ctrl+Space 标记，Enter 输出多行） | 列表 |
| 帮助浮层（? / F1，按模式裁剪键位表） | 全局 |
| docker / k8s / 单文件日志检索（默认跟随 tail -f 式 + 时间/条数筛选） | `scx-rg docker [名字]` 等子命令 |
| 通用 finder（stdin 管道 / docker-ps 预设） | `--provider` |
| 配置文件（防抖 / 忽略目录 / 主题三色） | `~/.config/scx-rg/config.toml` |
| OSC 52 剪贴板复制 / Ctrl+O 外部翻页器 | `Ctrl+Y` / `Ctrl+O` |

## 技术栈

| 依赖 | 用途 |
| --- | --- |
| Go 1.26（go.mod） | 单模块，无代码生成 |
| charm.land/bubbletea/v2 v2.0.9 + bubbles/v2 v2.2.0 | TUI 框架（Elm 架构 Update/View，声明式 tea.View）与 viewport/textinput 组件 |
| charm.land/lipgloss/v2 v2.0.6 | 样式/布局/计宽（Width 含边框；打印经 lipgloss.Println 降采样） |
| chroma/v2 v2.27.0 | 代码语法高亮 |
| mattn/go-sixel | sixel 图形编码 |
| golang.org/x/image | 图片缩放（CatmullRom）+ bmp/tiff/webp 解码 |
| muesli/termenv | 色彩档位探测与颜色降级（TrueColor→256→16） |
| charmbracelet/x/term | DA1 探测的 raw mode |
| BurntSushi/toml | 配置文件解析 |
| go-runewidth / reflow | 字符宽度（EastAsianWidth 保守口径）与折行/截断 |

外部命令依赖（均可选，缺失时降级而非崩溃）：

- **ripgrep（rg）**：内容模式必需；文件模式缺失时回退内置遍历
- **docker / kubectl**：仅对应子命令需要
- **fd / fzf 等**：仅 shell 集成示例（examples/）使用

## 里程碑状态

| 阶段 | 内容 | 状态 |
| --- | --- | --- |
| 初始框架 | TUI 骨架、kitty/sixel、搜索管线 | ✅ 2026-08-18 |
| M1 | 搜索质量：模糊评分、取消、流式渲染、ignore 规则 | ✅ |
| M2 | 预览增强：LRU 缓存、命中高亮、跨行 token 修复、CJK 宽度 | ✅ |
| M3 | 图片预览完善：halfblock、DA1 探测、kitty 清理链、GIF 首帧 | ✅（真机 checklist 见 README） |
| M4 | 交互与扩展：帮助、多选、finder、config.toml、shell 集成 | ✅ |
| M5 | 工程质量：golden frame、rg 解析单测、CI 补强 | ✅ |

详见 [docs/PLAN.md](../../docs/PLAN.md)。

Related: [architecture](architecture.md) · [README（用户文档）](../../README.md)
