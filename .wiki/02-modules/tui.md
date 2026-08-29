# tui —— 主界面编排层

包路径：`internal/tui`。bubbletea（Elm 架构）的 Model/Update/View 全部在这里，是唯一同时依赖 search / preview / logs 的编排层。

## 文件地图

| 文件 | 职责 |
| --- | --- |
| model.go | `Model` 结构与全部状态字段、消息类型定义、搜索/预览命令链（含防闪烁 `commitSearchResults`）、`drain` 测试 harness、`RenderOnce`（--once） |
| update.go | `Update` 消息分发 + `handleKey` 主键位表 |
| view.go | 帧布局：header / list / preview / status + 浮层分支；命中高亮 |
| help.go | 帮助浮层（? / F1），键位表按模式裁剪，宽/高自适应单双列 |
| styles.go | 全部 lipgloss 样式；`initStyles()` 唯一定义点 + `ApplyTheme` 主题注入 |
| action.go | 编辑器集成（Ctrl+E，`{file}/{line}` 模板 + nvim/vim/code/emacs/zed 预置、`$NVIM` quickfix） |
| copy.go | OSC 52 剪贴板（写 /dev/tty）、Ctrl+O 外部翻页器 |
| follow.go | `--follow` 文件跟随（搜索视图内）：800ms 轮询文件增长、`resultKey`（path:line）保位 |
| picker.go | docker/k8s 源选择器（Tab 多选 ≤4，Enter 进实时；`--snapshot` 单目标快照）+ `reenterPicker`（实时阶段 Ctrl+R 返回重选） |
| live.go | 实时多面板视图（docker/k8s 默认）：每面板一个 `logs.Stream` 流进程 + 100ms 批量管线 + tee 落盘；焦点面板滚动/跟随暂停恢复、y 复制搜索命令 |
| rangefilter.go | Ctrl+T 可视化筛选栏（时间/条数/Git 三段，客户端过滤不重抓；实时默认条数 50） |
| palette.go | 命令面板（`:` 输入为空时打开，模糊过滤全部命令） |
| history.go | 搜索历史：只记录实际使用过的查询，落盘 XDG state，Ctrl+G 浮层 |
| pipe.go | 管道输出（`\|`）：结果喂 `sh -c`，占位符替换，输出写回预览 |
| workspace.go | 多目录 workspace：额外搜索根（上限 8）、`~` 展开、目录输入浮层 |
| replace.go | AST 批量替换（ast-grep）：两段输入 → 匹配列表 y/a/n |
| blame.go | 状态栏 blame 摘要（Ctrl+B）：整文件 porcelain 按 mtime LRU 缓存 |
| gitlog.go | Git 历史搜索模式：`git log -G` 流式 + commit 详情 + Enter 复制 hash |
| gitfilter.go | Git 筛选（筛选栏第三段）：变更/暂存文件集拉取与路径过滤 |

## Model 状态分组（model.go）

| 分组 | 字段（节选） | 说明 |
| --- | --- | --- |
| 搜索 | `version`、`results`、`sel/offset`、`searching/searchErr`、`fallbackActive`、`staleList` | version 单调递增判废；`staleList`=新搜索已发起但结果未达（展示保留上一轮，防闪烁） |
| 流式 | `cancelSearch`、`streamCh` | cancel 立即杀 rg；streamCh 供 waitForResult 链继续消费 |
| 预览 | `vp`（bubbles viewport）、`prevPath/prevJump/prevKind/prevLang`、`prevCache`(LRU 32)、`renderFile`（可注入 fake）、`imgActive` | `imgActive` 驱动 kitty 图形清理链 |
| 模式 | `mode`(files/content)、`finder`、`picking`、`rangeBar`、`helpOverlay`、`gitLog` | 互斥的交互态，路由优先级见下 |
| 浮层 | `paletteOpen/paletteQuery/paletteSel`、`historyOpen/historySel`、`pipeOpen/pipeInput`、`dirOpen/dirInput`、`replaceOpen/replaceStage`、`astMode/astMatches` | 各独立浮层的开合与输入态（输入独立于主搜索框） |
| 多选 | `marked map[string]bool` | key = `resultKey(r)`（path:line），防筛选刷新错位 |
| 跟随/筛选 | `followSize/followKeep`、`filterDur/filterCap`、`capChosen`、`raw`（未过滤缓冲）、`tsOK`、`windowed`、`liveTicking` | 搜索视图的日志场景专用；`capChosen`=用户手动选过条数档（实时默认不再覆盖） |
| 实时多面板 | `liveMode/liveFocus/livePanels`、`liveCh/liveCancel/liveSeq`、`pickerMarks` | `livePanels` 每面板 `buf/vp/follow/exited`（仅 Update goroutine 读写）；`liveSeq` 防重进实时后旧管线消息串扰 |
| Git | `gitFilter/gitAllow/gitKnown/gitOK/gitLoading`、`blameOn/blameText/blameCache`、`extraRoots` | 筛选栏第三段 / blame 摘要 / 多目录 |
| 注入点 | `renderFile`、`writeClipboard`、`now`、`cfg.ListSources/FetchLog/StreamLog/GitFiles/BlameFetch/PipeRun/GitShow/NvimSend/AstScan/AstApply/GitClean` | 测试替换 fake 的钩子（`StreamLog` nil 时用 `logs.Stream`；`LiveDir/LiveTargets/LivePick` 驱动实时入口） |

## 按键路由优先级（handleKey，update.go）

```text
0. m.liveMode     → handleLiveKey（实时多面板独占：焦点滚动/切换、y 复制、
                    Ctrl+R 重选、? 帮助接管）
1. m.picking      → handlePickerKey（源选择器独占：Tab 多选、Enter 进实时/快照）
2. m.rangeBar     → handleRangeBarKey（筛选栏聚焦）
3. paletteOpen / historyOpen / pipeOpen / dirOpen / replaceOpen / astMode
                  → 各浮层按键处理（互斥，按此顺序短路）
4. m.helpOverlay  → 任意键关闭（Ctrl+C 仍直接退出）
5. 空输入和弦族：? / F1 帮助、: 命令面板、| 管道、R AST 替换
6. 主键位表：Ctrl+C / Enter / Esc / Ctrl+Space(ctrl+@) / Ctrl+T / Ctrl+R(pickerKind≠"")
   / Ctrl+F / Ctrl+O / Ctrl+E / Ctrl+Y / Ctrl+G / Ctrl+B / Tab(finder 禁用) /
   ↑↓ / PgUp/PgDn(图片预览禁滚)
7. default → textinput（输入变化 → 防抖重搜）
```

Esc 是递进语义：输入非空清输入 → 标记非空清标记 → 才退出。
Enter 输出：`pickedOutput()`——有标记按列表顺序输出全部标记项（全部被过滤则退回当前选中），否则当前选中；`PickLine=true` 输出原行文本（finder/日志），否则绝对路径（gitLog 模式改为复制 commit hash）。

## 防闪烁：staleList + commitSearchResults（model.go）

`runSearch` 发起新搜索时**不再立即清空**列表/预览，只置 `staleList=true`——旧结果保持可见，状态栏显示「* 搜索中」。新一轮的**首个回包**（`resultsMsg` / `resultMsg` / `streamDoneMsg` 三处入口）先调 `commitSearchResults()` 原子换下上一轮展示（清 results/raw/tsOK/sel/offset/预览），再走各自的原有逻辑；零命中流在 streamDoneMsg 清掉旧列表，不残留。动机：防抖到期与跟随模式 800ms 刷新都走 `runSearch`，先清后填会造成「清空 → 搜索中 → 回填」的整屏闪烁。

## 消息链

| 消息 | 生产者 | 消费 |
| --- | --- | --- |
| `debounceMsg{version}` | tickDebounce | 版本对齐则 runSearch |
| `resultsMsg` | 同步 provider | `commitSearchResults` → 覆盖 results + refilter + 零命中回退判定 |
| `resultMsg` / `streamDoneMsg` | waitForResult 链 | 首条到达先 `commitSearchResults`；流式逐条追加 / 收尾（windowed 重算） |
| `previewMsg{path, rendered}` | 渲染 goroutine | `path != prevPath` 丢弃（用户已切走） |
| `pickerLoadedMsg` / `snapshotReadyMsg` / `followTickMsg` / `liveTickMsg` | picker/快照/--follow 轮询/实时滑窗 | 各自状态推进 |
| `liveLinesMsg{seq,panel,lines}` / `liveDoneMsg{seq,panel,err}` | live.go 批量管线（100ms 窗口聚合）/ Stream 进程收束 | seq 不匹配丢弃；追加缓冲+贴底 rebuild / 面板标 ■ 或显错 |
| `gitFilesMsg` | loadGitFiles | gitOK 翻转 → 筛选栏第三段可见性 + 面板重排 |
| `blameMsg` / `commitDetailMsg` | blame/gitlog 异步拉取 | 状态栏摘要 / 预览详情（过期丢弃） |
| `astScanMsg` / `astAppliedMsg` / `pipeDoneMsg` / `editorDoneMsg` / `pagerDoneMsg` / `nvimDoneMsg` | ast-grep/管道/编辑器/翻页器 | 结果写回 / 错误提示 |

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
