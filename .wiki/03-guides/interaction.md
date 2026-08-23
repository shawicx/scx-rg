# 交互指南：键位 / 多选 / finder / shell 集成

键位的事实来源是 `internal/tui/help.go`（`helpGroups()`，按模式裁剪）与 `update.go` 的 `handleKey`——本页与代码同步维护。

## 主模式键位表

| 键 | 行为 | 备注 |
| --- | --- | --- |
| 直接输入 | 实时搜索（200ms 防抖，可 config 调） | 过期结果按 version 判废 |
| `Esc` | 递进：清输入 → 清标记 → 退出 | |
| `Tab` | 文件 ⇄ 内容模式 | finder 模式禁用 |
| `Ctrl+F` | 精确/模糊（文件）· 字面量/正则（内容） | 状态栏徽章显示当前档 |
| `↑ ↓` / `Ctrl+P Ctrl+N` | 移动选中 | 预览跟随 |
| `Ctrl+Space` | 标记/取消当前行并下移 | 终端发送 NUL，bubbletea 识别为 `ctrl+@` |
| `Enter` | 输出选中；**有标记输出全部标记项（多行）** 到 stdout 后退出 | PickLine 模式（finder/日志）输出行文本 |
| `PgUp PgDn` | 预览滚动半页 | 图片预览禁用（图形不随文本滚动） |
| `Ctrl+Y` | 复制当前预览（OSC 52） | 日志/finder 模式复制行文本 |
| `Ctrl+O` | 外部翻页器打开（自由复制） | 图片预览不可用 |
| `Ctrl+T` | 结果筛选栏（时间窗/条数） | 日志/内容模式 |
| `?`（输入为空）/ `F1` | 帮助浮层 | 任意键关闭 |
| `Ctrl+C` | 退出（杀后台 rg/跟随进程） | 任何浮层下有效 |

picker 模式补充：`Ctrl+R` 刷新源列表、Enter 抓取；筛选栏内：`↑↓/Tab` 切段、`←→` 移动。

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

## docker / k8s / 日志子命令

```bash
scx-rg docker [名字] [--snapshot]   # 默认实时跟随（tail 10万行 + 持续追加）
scx-rg k8s [Pod] [-n ns] [-c 容器]
scx-rg --follow /var/log/app.log    # 本地文件跟随
```

无名字进入源选择器（左列表右详情）；日志模式 Enter 输出选中行文本。

Related: [tui（实现）](../02-modules/tui.md) · [search（finder 后端）](../02-modules/search.md) · [README（用户视角）](../../README.md)
