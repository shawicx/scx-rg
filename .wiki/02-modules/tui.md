# tui —— 主界面编排层

包路径：`internal/tui`。bubbletea（Elm 架构）的 Model/Update/View 全部在这里，是唯一同时依赖 search / preview / logs 的编排层。

## 文件地图

| 文件 | 职责 |
| --- | --- |
| model.go | `Model` 结构与全部状态字段、消息类型定义、搜索/预览命令链、`drain` 测试 harness、`RenderOnce`（--once） |
| update.go | `Update` 消息分发 + `handleKey` 主键位表 |
| view.go | 帧布局：header / list / preview / status + 帮助浮层分支；命中高亮 |
| help.go | 帮助浮层（? / F1），键位表按模式裁剪，宽/高自适应单双列 |
| styles.go | 全部 lipgloss 样式；`initStyles()` 唯一定义点 + `ApplyTheme` 主题注入 |
| copy.go | OSC 52 剪贴板（写 /dev/tty）、Ctrl+O 外部翻页器 |
| follow.go | 日志跟随模式：800ms 轮询文件增长、`resultKey`（path:line）保位 |
| picker.go | docker/k8s 源选择器（两阶段的第一阶段） |
| rangefilter.go | Ctrl+T 可视化筛选栏（时间窗/条数，客户端过滤不重抓） |

## Model 状态分组（model.go）

| 分组 | 字段（节选） | 说明 |
| --- | --- | --- |
| 搜索 | `version`、`results`、`sel/offset`、`searching/searchErr`、`fallbackActive` | version 单调递增，防抖与过期结果判废的依据 |
| 流式 | `cancelSearch`、`streamCh` | cancel 立即杀 rg；streamCh 供 waitForResult 链继续消费 |
| 预览 | `vp`（bubbles viewport）、`prevPath/prevJump/prevKind/prevLang`、`prevCache`(LRU 32)、`renderFile`（可注入 fake）、`imgActive` | `imgActive` 驱动 kitty 图形清理链 |
| 模式 | `mode`(files/content)、`finder`、`picking`、`rangeBar`、`helpOverlay` | 互斥的交互态，路由优先级见下 |
| 多选 | `marked map[string]bool` | key = `resultKey(r)`（path:line），防筛选刷新错位 |
| 跟随/筛选 | `followSize/followKeep`、`filterDur/filterCap`、`raw`（未过滤缓冲）、`tsOK`、`windowed` | 日志场景专用 |
| 注入点 | `renderFile`、`writeClipboard`、`now`、`cfg.ListSources/FetchLog/FollowLog` | 测试替换 fake 的钩子 |

## 按键路由优先级（handleKey，update.go）

```text
1. m.picking      → handlePickerKey（源选择器独占）
2. m.rangeBar     → handleRangeBarKey（筛选栏聚焦）
3. m.helpOverlay  → 任意键关闭（Ctrl+C 仍直接退出）
4. ? (输入为空时) / F1 → 打开帮助浮层
5. 主键位表：Ctrl+C / Enter / Esc / Ctrl+Space(ctrl+@) / Ctrl+T / Ctrl+F /
   Ctrl+O / Ctrl+Y / Tab(finder 禁用) / ↑↓ / PgUp/PgDn(图片预览禁滚)
6. default → textinput（输入变化 → 防抖重搜）
```

Esc 是递进语义：输入非空清输入 → 标记非空清标记 → 才退出。
Enter 输出：`pickedOutput()`——有标记按列表顺序输出全部标记项（全部被过滤则退回当前选中），否则当前选中；`PickLine=true` 输出原行文本（finder/日志），否则绝对路径。

## 消息链

| 消息 | 生产者 | 消费 |
| --- | --- | --- |
| `debounceMsg{version}` | tickDebounce | 版本对齐则 runSearch |
| `resultsMsg` | 同步 provider | 覆盖 results + refilter + 零命中回退判定 |
| `resultMsg` / `streamDoneMsg` | waitForResult 链 | 流式逐条追加 / 收尾 |
| `previewMsg{path, rendered}` | 渲染 goroutine | `path != prevPath` 丢弃（用户已切走） |
| `pickerLoadedMsg` / `snapshotReadyMsg` / `followTickMsg` / `liveTickMsg` / `pagerDoneMsg` | picker/跟随/翻页 | 各自状态推进 |

## kitty 图形清理链（imgActive，M3）

kitty overlay 图形不占字符流、不随文本替换消失，残留靠四层机制根治：

1. `kittyBlock`（preview 包）每次输出前发删除序列 `a=d,d=a,i=7`（幂等）
2. `setPreviewContent`：从含 `\x1b_G` 的内容切到不含时注入删除前缀
3. `previewView` 空态分支：预览被清空时把删除序列缀在提示文本前（一次性消费 imgActive）
4. `main` 退出时写 stdout `preview.KittyDeleteAll` 清全部图形

## drain —— 测试 harness（model.go）

`drain(cmd)` 同步驱动 cmd/msg 链直到结束：展开 `tea.BatchMsg`、丢弃 `cursor.BlinkMsg`（自续链会死循环）、上限 2^20 次。全部 tui 测试用它模拟事件循环，配合 `m.renderFile` 等 fake 注入实现无终端测试。`RenderOnce` 是它之上的 --once 路径（不开事件循环渲一帧，CI 冒烟也用它）。

## finder 模式（M4）

`Config.Candidates` 非空即进入：`provider()` 返回 `search.ListProvider`；Tab 与全文回退禁用；预览先判候选是否真实文件路径（`fd | scx-rg --provider stdin` 场景），是则正常异步预览，否则同步详情面板（行文本 + Detail）。

Related: [architecture](../01-overview/architecture.md) · [search](search.md) · [preview](preview.md) · [interaction（键位）](../03-guides/interaction.md) · [testing（drain 用法）](../03-guides/testing.md)
