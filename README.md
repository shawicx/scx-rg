# scx-rg

终端里的实时搜索 + 预览工具。Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss)。

## 功能

- **实时防抖搜索**：输入即搜（默认 200ms 防抖），过期结果自动丢弃（版本号判废）
- **双模式**：`Tab` 切换
  - 文件模式：fzf 式模糊匹配（空格分词 AND、边界/连续加权评分排序，命中字符高亮）；优先用 `rg --files` 枚举（尊重 .gitignore），无 rg 时回退内置遍历
  - 内容模式：`rg --json` 流式解析，结果边搜边出；输入变化立即杀掉上一轮 rg 进程
- **多面板预览**：左侧结果列表、右侧预览面板
  - 代码：chroma 语法高亮 + 行号槽，内容模式自动跳转到匹配行
  - 图片：kitty 图形协议 / sixel 协议直接在终端内渲染，不支持时显示占位提示
- **fzf 式工作流**：`Enter` 退出并把选中文件绝对路径打印到 stdout，可接管道

## Docker / Kubernetes / 服务器日志检索

```bash
scx-rg docker                        # 交互选择容器（模糊过滤，免记名字）
scx-rg docker <容器名>                # 直达：抓取最近 100000 行日志快照，进入全文检索
scx-rg docker --follow               # 选择容器后 tail -f 式实时追新
scx-rg docker <容器名> --follow       # 直达 + 实时追新

scx-rg k8s                           # 交互选择 Pod
scx-rg k8s <Pod名> [-n namespace] [-c 容器] [--follow]

scx-rg --follow /var/log/app.log     # 本地服务器日志实时跟随
```

### 容器 / Pod 选择器

`scx-rg docker` / `scx-rg k8s` 不带名字时进入选择器：

- 左侧列出全部容器（名称 · 镜像 · 状态，`Up`/`Running` 绿色标注）或 Pod（名称 · namespace · `就绪数/总数 状态`），右侧显示选中目标详情
- 输入即模糊过滤（名称+镜像/namespace），`↑↓` 选择，`Ctrl+R` 刷新列表
- `Enter` 抓取该目标最近 100000 行日志，无缝切入全文检索界面（配合过 `--follow` 则改为实时跟随）
- 抓取失败（如 daemon 未启动）会显示错误并停留在选择器，可重试或换目标

- 跟随模式：日志持续写入快照，界面每 800ms 检测增长并自动重跑当前查询；**保持你的选中位置**（path:line 对齐），状态栏显示「⟳ 跟随 · 大小」
- **Ctrl+T 可视化筛选**：「过去 15 分钟」「最近 100 条」等常用参数直接在界面上选，即时生效，详见下方按键表
- 快照（`docker logs` / `kubectl logs --timestamps --tail`）带时间戳；`Enter` 把选中的日志行文本输出到 stdout；快照文件退出自动清理
- **大日志窗口化预览**：超过 1MB 的文件不再拒绝预览，只渲染命中行前后 40/80 行的上下文窗口（真实行号 + `⋯` 跳过标记 + 长行折行），搜索命中自动定位
- 搜索错误（非法正则、权限问题等）显示在状态栏；rg 退出码 1（无匹配）不误报为错误

## 构建与运行

```bash
go build -o scx-rg .

# 交互运行
./scx-rg                     # 当前目录，文件模式
./scx-rg -path ~/code -mode content

# 配合管道
vim "$(./scx-rg -mode content -q TODO)"
```

内容模式需要安装 [ripgrep](https://github.com/BurntSushi/ripgrep)（`brew install ripgrep`）。

## 按键

| 按键 | 作用 |
| --- | --- |
| 输入 | 实时搜索（防抖） |
| `↑` `↓` / `Ctrl+P` `Ctrl+N` | 选择结果 |
| `Tab` | 切换 文件/内容 模式 |
| `Ctrl+T` | 打开可视化筛选栏（时间范围 / 条数封顶） |
| `PgUp` `PgDown` | 滚动预览 |
| `Enter` | 选定（stdout 输出路径）并退出 |
| `Esc` | 清空搜索词；已空则退出 |
| `Ctrl+C` | 退出 |

### 可视化筛选栏（Ctrl+T）

```
⏱ 时间   全部  1分钟  5分钟  15分钟  1小时  6小时  24小时
⇥ 条数   全部  100条  500条  5000条
```

- `←` `→` 在预设间移动光标，**即时生效**；`↑` `↓`/`Tab` 切换两段；`Enter`/`Esc` 收起
- **时间**：只显示行首时间戳在「过去 X 分钟/小时」内的行（自动识别 docker/kubectl 快照、`2026-08-20 10:00:00`、nginx、syslog 等常见格式）；无时间戳的行（多行堆栈续行）保留；检测不到时间戳时该段失效并提示
- **条数**：只保留最新的 N 条命中（配合搜索词就是「最近 100 条 ERROR」）
- 全部在客户端完成，不重新抓取日志；跟随模式（--follow）下每次刷新自动重新过滤
- 生效的筛选显示在底部状态栏，如 `⏱ 15分钟 · 末100条`

## 参数

```
-path string    搜索根目录（默认 .）
-mode string    files | content（默认 files）
-img string     auto | kitty | sixel | none（默认 auto，按环境变量探测）
-debounce-ms    搜索防抖间隔（默认 200）
-once           渲染一帧后退出（调试/CI 冒烟）
-q string       配合 --once 模拟搜索词
-preview-file   配合 --once 强制预览指定文件
-w / -h         配合 --once 的渲染尺寸
```

## 架构

```
main.go                     入口：参数解析、协议探测、程序启动
internal/
  search/
    provider.go             Provider / SyncProvider / StreamProvider 接口
    fuzzy.go                模糊匹配与评分（子序列 + 边界/连续加权）
    files.go                文件名搜索：rg --files / 内置遍历 + 模糊排序
    rg.go                   ripgrep --json 流式解析（可取消）
  preview/
    preview.go              Render 入口：按扩展名分发
    code.go                 chroma 高亮 + 行号槽 + 匹配行标记
    image.go                kitty/sixel 图形协议编码
    cellsize_unix.go        TIOCGWINSZ 取单元格像素尺寸
    protocol.go             图形协议探测（环境变量启发式）
  tui/
    model.go                状态 + 消息定义 + 搜索/预览命令与流式消费链
    update.go               事件处理（按键、防抖到期、流式/同步回包）
    view.go                 布局渲染（头部/列表/预览/状态栏 + 命中高亮）
    styles.go               Lipgloss 样式
testdata/demo.png           图片预览测试素材
```

## 测试

```bash
go test ./...
```

- `internal/search`：模糊匹配评分/分词语义、rg --files 的 gitignore 行为、流式搜索与取消不泄漏
- `internal/tui`：流式结果追加、过期消息丢弃、新搜索重置状态（通过 drain 驱动 cmd 链模拟事件循环）

## 已知限制 / Roadmap

- IDE 运行窗/管道等无 TTY 环境会自动降级为单帧渲染（提示后以 `--once` 效果输出），交互模式请在真实终端运行
- 图片协议探测靠环境变量启发式；sixel 精确探测需 DA1 查询（待实现）
- 预览区滚动大图时图形序列可能被 viewport 切分（待验证/优化）
- 文件模式为子串匹配，后续可换模糊匹配 + 排序评分
- 列表无虚拟化（当前上限 500 条，够用）
- 预览长行按面板宽度折行显示（行号只在首段），单行最多折 10 段、超出以 ⋯ 标记（防超长 JSON 行撑爆视口）
