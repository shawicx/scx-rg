# 交互指南：键位 / 多选 / finder / shell 集成

键位的事实来源是 `internal/tui/help.go`（`helpGroups()`，按模式裁剪）与 `update.go` 的 `handleKey`——本页与代码同步维护。

## 主模式键位表

| 键 | 行为 | 备注 |
| --- | --- | --- |
| 直接输入 | 实时搜索（200ms 防抖，可 config 调） | 过期结果按 version 判废；搜索中旧列表保持可见（`staleList` + `commitSearchResults`，见 tui 页） |
| `Esc` | 递进：清输入 → 清标记 → 退出 | |
| `Tab` | 文件 ⇄ 内容模式 | finder 模式禁用 |
| `Ctrl+F` / `Alt+F` | 精确/模糊（文件）· 字面量/正则（内容） | 状态栏徽章显示当前档 |
| `↑ ↓` / `Ctrl+P Ctrl+N` / `Alt+P Alt+N` | 移动选中 | 预览跟随 |
| `Ctrl+Space` / `Alt+M` | 标记/取消当前行并下移 | 终端发送 NUL，bubbletea 识别为 `ctrl+@` |
| `Enter` | 输出选中；**有标记输出全部标记项（多行）** 到 stdout 后退出 | PickLine 模式（finder/日志）输出行文本 |
| `PgUp PgDn` | 预览滚动半页 | 图片预览禁用（图形不随文本滚动） |
| `Ctrl+Y` / `Alt+Y` | 复制当前预览（OSC 52） | 日志/finder 模式复制行文本 |
| `Ctrl+O` / `Alt+O` | 外部翻页器打开（自由复制） | 图片预览不可用 |
| `Ctrl+T` / `Alt+T` | 结果筛选栏（时间窗/条数/Git） | 见下方筛选栏 |
| `Ctrl+R` / `Alt+R` | docker/k8s 会话：**返回选择器重选容器/Pod**（`reenterPicker`） | 选择器内同键=刷新列表 |
| `Ctrl+G` / `Alt+G` | 搜索历史浮层（Enter 回填执行 · Del 删除） | history.go，落盘 XDG_STATE_HOME |
| `Ctrl+B` / `Alt+B` | 状态栏 blame 摘要开关 | blame.go，git 仓库内 |
| `Ctrl+E` / `Alt+E` | 编辑器打开选中文件到对应行 | 需 config `[editor]`；有 `$NVIM` 发 quickfix |
| `:`（输入为空） | 命令面板（全部命令的无冲突入口） | palette.go |
| `\|`（输入为空） | 结果喂给外部命令 `{path} {line} {text}` | pipe.go，输出写回预览 |
| `R`（输入为空） | AST 批量替换（ast-grep，需干净工作区） | replace.go |
| `?`（输入为空）/ `F1` | 帮助浮层 | 任意键关闭 |
| `Ctrl+C` | 退出（杀后台 rg/跟随进程） | 任何浮层下有效 |

所有 Ctrl 功能键都有 `Alt+字母` 别名（堡垒机/浏览器 Web 终端常截获 Ctrl 组合键）。

picker 模式补充：输入过滤、`Ctrl+R` 刷新源列表、Enter 抓取；检索阶段 `Ctrl+R` 返回选择器。筛选栏内：`↑↓/Tab` 切段、`←→` 移动并即时生效。

## 多选语义

- 标记 key = `path:line`（`resultKey`），查询过滤后标记不丢；输出时按当前列表顺序、被过滤掉的标记跳过，**全部失效则退回当前选中**
- 状态栏显示「已标记 N」；列表行前缀 `✓`（绿色）
- 输出多行走 stdout（`printPicked` 单次 Println 多行字符串），天然支持 `$(...)` 命令替换

## 通用 finder（--provider）

```bash
fd --type f | scx-rg --provider stdin         # 候选是真实路径 → 自动获得文件预览
git branch | scx-rg --provider stdin          # 任意行候选 → 详情面板
scx-rg --provider docker-ps                   # 内置容器列表（输出容器名）
docker stop $(scx-rg --provider docker-ps)    # 组合
```

finder 特性：本地模糊过滤（与文件模式同一套评分）、Tab/全文回退禁用、Ctrl+Space 多选、Enter 输出原行文本。stdin 是终端（没接管道）时报用法退出；候选上限 10 万行。

## shell 集成（examples/）

`source examples/scx-rg.zsh`（或 fish 版）获得 fzf 式键绑定：

- **CTRL-T**：`fd --type f | scx-rg --provider stdin` 选文件插入命令行
- **CTRL-R**：zsh `fc -l 1` / fish `builtin history` 喂 stdin provider 搜历史，选中替换命令行

zsh 版已过 `zsh -n` 语法校验；fish 需 fish 3.x。

## 可视化筛选栏（Ctrl+T，rangefilter.go）

```text
时间   实时  全部  1分钟  5分钟  15分钟  1小时  6小时  24小时
条数   全部  20条  50条  100条  500条  5000条
Git    全部  仅改动  仅暂存   （git 仓库内第三段）
```

- 时间=「实时」是 30 秒滑动窗口（跟随模式下每秒重算，旧行随时间滚出）
- **时间切实时且条数停在「全部」（且用户从未在条数段手动选档，`capChosen`）时，条数自动收窄为 50 条**——实时窗全量命中会刷屏
- 时间过滤只丢弃「带时间戳且早于范围」的行（`parseLineTime` 识别 RFC3339/nginx/syslog 等格式）；无时间戳行保留；`tsOK=false` 时段失效并提示

## docker / k8s / 日志子命令

```bash
scx-rg docker [名字] [--snapshot]   # 默认实时跟随（tail 10万行 + 持续追加）
scx-rg k8s [Pod] [-n ns] [-c 容器]
scx-rg --follow /var/log/app.log    # 本地文件跟随
```

无名字进入源选择器（左列表右详情）；日志模式 Enter 输出选中行文本；检索阶段 `Ctrl+R` 随时返回选择器重选目标（`reenterPicker`：停搜索与跟随进程、清检索态、重载源列表）。

Related: [tui（实现）](../02-modules/tui.md) · [search（finder 后端）](../02-modules/search.md) · [README（用户视角）](../../README.md)
