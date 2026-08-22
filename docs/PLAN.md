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

## M3 图片预览完善（≈1–2 天，需真终端）

- **真机实测**：kitty / ghostty / wezterm 下跑 `testdata/demo.png`，验证 alt-screen 重绘、滚动、选中切换时图像是否残留/错位（嵌入 viewport 的方案最大的不确定性在此）
- **halfblock 降级**：第三档渲染——纯文本半块字符 + truecolor（任何终端可用，参考 timg），作为 kitty/sixel 都不可用时的兜底，替代现在的纯文字占位
- **DA1 精确探测**：发 `ESC[c` 查询 sixel 能力（raw mode + 超时读），替换环境变量启发式
- **GIF 首帧**：当前 image.Decode 已能解，确认显示即可；动画播放列入 backlog 不做

## M4 交互与扩展（渐进）

- `?` 帮助浮层（完整键位表）
- 多选（`Mark` 键 + Enter 输出多行路径）
- stdin Provider：`scx-rg --provider stdin` 读候选，向通用 finder 演进的第一步
- 预设 Provider：git 分支、docker ps、历史命令等
- 配置文件 `~/.config/scx-rg/config.toml`：主题色、防抖时长、忽略规则
- zsh/fish 集成示例（CTRL-T 唤起）

## M5 工程质量（随 M1 同步启动，不后置）

- 单测：`search`（rg JSON 解析、walk 跳过规则）、`preview`（高亮行数对齐、截断、二进制嗅探）、`tui`（用 tea 的 test harness 驱动 Update 状态机）
- **golden frame 测试**：`--once` 输出去 ANSI 后做快照对比（现有 `--once` 就是为这个设计的）
- CI：GitHub Actions（build + test + vet），goreleaser 发布

## 建议的第一周切入点（按序）

1. M1 的搜索取消 + 文件模式模糊匹配（体感提升最大的两件事）
2. M3 的真终端图片实测（最早暴露架构级风险，越晚发现返工越大）
3. M5 的 golden frame 快照测试（给后续所有改动兜底）

## 风险与待验证

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| viewport 嵌图形序列在真终端的行为 | 图片功能可能需独立渲染层 | M3 最优先实测；备选方案 halfblock |
| 大文件高亮卡 UI（当前在 goroutine，但 1MB 上限仍慢） | 切选卡顿 | 预览缓存 + 上限调优 + 大文件只渲染可视区 |
| bubbletea v2 正式发布 | API 迁移成本 | 锁 v1，升级列入 backlog |
