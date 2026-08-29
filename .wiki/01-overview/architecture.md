# 架构

## 总体形态

单二进制、单入口（`main.main`，全仓唯一 entry point），五个内部包分层清晰。以下模块边界数据来自 codebase graph 实测（调用边数）：

```text
main ──4──▶ tui ──22──▶ preview
 │           │──12──▶ search
 │           │──3───▶ logs
 ├──2──▶ preview        ┌── 功能域（被依赖）
 ├──1──▶ config    search / preview / logs（高 fan-in、低 fan-out）
 └──1──▶ runLogSource ──4──▶ logs
                   tui（编排层，fan-out 37）
                   main（入口，只出不进）
                   config（叶子，只进不出）
```

| 包 | 层（graph 判定） | 职责 |
| --- | --- | --- |
| `main` | entry | flag 解析、config 加载、协议探测、provider 分发、tea 启动、docker/k8s 子命令 |
| `internal/tui` | internal（编排） | bubbletea Model/Update/View 全部状态机；是唯一同时依赖 search/preview/logs 的包 |
| `internal/search` | core | Provider 抽象 + 文件枚举 + rg 流式 + 模糊评分 |
| `internal/preview` | core | 文件→可进 viewport 的 ANSI 内容（代码高亮 / 图片编码）+ 渲染缓存 |
| `internal/logs` | core | docker/kubectl 日志源列表、快照抓取与实时流（`Stream` tee 落盘 + `LivePath` 稳定路径） |
| `internal/config` | leaf | ~/.config/scx-rg/config.toml 读取（防抖/忽略/主题） |

依赖方向纪律：search 与 preview 互不感知、不反向依赖 tui（graph 中 `search→tui` 的 2 条边均为测试文件）。

## 目录地图

```text
main.go                  入口与子命令分派；version.go 由 goreleaser ldflags 注入
internal/
  search/                provider.go(接口) files.go(枚举+打分) rg.go(流式) fuzzy.go(评分) list.go(finder)
  preview/               preview.go(分发) code.go(高亮) image.go(图形协议) halfblock.go(第三档)
                         protocol.go(探测) da1_unix.go(DA1查询) cache.go(LRU) cellsize_*.go(像素)
  tui/                   model.go(状态+消息) update.go(事件) view.go(布局) styles.go(样式+主题)
                         help.go(帮助浮层) copy.go(OSC52/翻页) follow.go(--follow 文件跟随轮询)
                         picker.go(源选择器) live.go(实时多面板) rangefilter.go(Ctrl+T 筛选)
  logs/                  sources.go(ListSources) docker.go(快照/流参数) stream.go(Stream+LivePath)
  config/                config.go
examples/                scx-rg.zsh / scx-rg.fish（CTRL-T / CTRL-R 集成）
testdata/                demo.png（图片实测素材）；golden 基线在 internal/tui/testdata/golden/
```

## 核心调用链

### 1. 搜索链（输入 → 结果）

```text
tea.KeyMsg → Model.handleKey (update.go)
  → 输入变化: version++ → tickDebounce(200ms) → debounceMsg → runSearch
  → runSearch: cancelSearch 杀旧 rg → staleList=true（旧列表保持可见，防闪烁）
               → 按 provider 分流
      files/finder(同步): FilesProvider/ListProvider.Search → resultsMsg
      content(流式): RipgrepProvider.SearchStream → waitForResult 逐条 → resultMsg
                       channel 关闭 → streamDoneMsg
  → 首个回包(resultsMsg/resultMsg/streamDoneMsg): commitSearchResults 原子换下旧列表
  → resultsMsg: refilter(false) 应用客户端筛选 → followSelection
  → 文件名零命中(content 除外): startFallbackStream 自动全文回退
```

同步 provider 有 10s 超时（model.go runSearch 内 ctx）；流式取消靠 ctx.Done 同时杀 rg 进程与解除 channel 发送阻塞。

### 2. 预览链（选中 → 面板）

```text
followSelection (model.go)
  → 同文件同行: 跳过（免重渲）
  → finder 模式: finderPath 判定——是真实文件路径走 renderSelectionPreview，
                 否则同步显示详情面板（无 IO 不异步）
  → renderSelectionPreview: prevCache.Get 命中→applyPreview 同步
                            未命中→goroutine renderFile→cache.Put→previewMsg→applyPreview
  → applyPreview: setPreviewContent（kitty 图形切走时注入删除序列）
                  → 按 JumpOffset 滚动定位
```

### 3. 启动链（main）

```text
main → config.Load（flag 显式 > config.toml > 默认）
     → tui.ApplyTheme（initStyles 重建包级样式）
     → preview.ParseProtocol/Detect（env kitty 系 → DA1 sixel → TERM 启发式 → halfblock）
     → --provider 分支: loadCandidates（stdin 读管道 / docker-ps 走 logs.ListSources）
     → tui.New → --once? RenderOnce 单帧 : tea.NewProgram(AltScreen)
     → p.Run 返回后 clearKittyGraphics（退出清图形）
```

### 4. 实时链（docker/k8s 子命令默认路径）

```text
main.runLogSource → Available 预检 → LiveDir=UserCacheDir/scx-rg/logs（MkdirAll）
  → 有名字: LiveTargets 直达单面板；无名字: 选择器 Tab 多选 ≤4 → Enter
  → tui.startLive([]logs.Target)：每面板一个 logs.Stream 进程（logs -f --timestamps --tail）
      行回调 → linesCh → batcher 100ms 批量窗口 → liveLinesMsg（100ms/200 行先到先冲）
      进程收束 → liveDoneMsg（err=nil 容器停止；err!=nil 启动失败——stderr 已合流入面板）
      同时 tee 落盘 LivePath（O_TRUNC 起笔，bufio 随批刷新）
  → liveView 分屏渲染（1/2/3/4 面板）→ Ctrl+R reenterPicker / Ctrl+C stopLive 清场
  → 落盘文件留给默认 scx-rg 命令检索（--follow 边跟边搜，走 2 号搜索链 + follow 轮询）
```

实时与搜索不共享视图状态，仅共享选择器与 logs 包；`--snapshot` 仍走「快照 → 搜索链」旧路径。

## 热点函数（graph fan-in 实测）

| 函数 | fan-in | 说明 |
| --- | --- | --- |
| `tui.Model.Update` | 51 | 全部消息的入口分发——改交互行为必经之地 |
| `preview.Render` | 39 | 预览唯一入口（代码/图片分发） |
| `tui.Model.drain` | 34 | 测试 harness：同步驱动 cmd/msg 链模拟事件循环 |
| `tui.New` | 31 | 模型构造（mode/finder/picker 初始化） |
| `tui.Model.View` | 25 | 帧布局（header/list/preview/status + helpOverlay 分支） |
| `search.Fuzzy` | 14 | 模糊评分核心 |

Related: [project-overview](project-overview.md) · [tui](../02-modules/tui.md) · [search](../02-modules/search.md) · [preview](../02-modules/preview.md)
