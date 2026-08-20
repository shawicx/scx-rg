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

## Docker / 服务器日志检索

```bash
scx-rg docker              # 列出容器（透传 docker ps）
scx-rg docker <容器名>      # 抓取最近 100000 行日志快照，直接进入全文检索
```

- 抓取完成即进入内容模式，输入关键词实时流式检索；`Enter` 把选中的日志行文本输出到 stdout
- 头部标题显示 `docker:<容器名>`；快照文件在退出时自动清理
- **大日志窗口化预览**：超过 1MB 的文件（服务器日志同样适用，`scx-rg -path /var/log -mode content`）不再拒绝预览，而是只渲染命中行前后 40/80 行的上下文窗口，保留真实行号与 `⋯` 跳过标记，搜索命中自动定位
- 搜索错误（非法正则、权限问题等）会显示在状态栏，不再静默返回空结果

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
| `PgUp` `PgDown` | 滚动预览 |
| `Enter` | 选定（stdout 输出路径）并退出 |
| `Esc` | 清空搜索词；已空则退出 |
| `Ctrl+C` | 退出 |

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
