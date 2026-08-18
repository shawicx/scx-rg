# scx-rg

终端里的实时搜索 + 预览工具。Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss)。

## 功能

- **实时防抖搜索**：输入即搜（默认 200ms 防抖），过期结果自动丢弃（版本号判废）
- **双模式**：`Tab` 切换
  - 文件模式：文件名子串匹配（内置遍历，跳过 `.git`/`node_modules` 等）
  - 内容模式：调用 `rg --json` 全文搜索，结果定位到行
- **多面板预览**：左侧结果列表、右侧预览面板
  - 代码：chroma 语法高亮 + 行号槽，内容模式自动跳转到匹配行
  - 图片：kitty 图形协议 / sixel 协议直接在终端内渲染，不支持时显示占位提示
- **fzf 式工作流**：`Enter` 退出并把选中文件绝对路径打印到 stdout，可接管道

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
    provider.go             Provider 接口（Result{Path, Line, Text}）
    walk.go                 文件名遍历搜索
    rg.go                   ripgrep --json 流式解析
  preview/
    preview.go              Render 入口：按扩展名分发
    code.go                 chroma 高亮 + 行号槽 + 匹配行标记
    image.go                kitty/sixel 图形协议编码
    cellsize_unix.go        TIOCGWINSZ 取单元格像素尺寸
    protocol.go             图形协议探测（环境变量启发式）
  tui/
    model.go                状态 + 消息定义 + 防抖/搜索/预览命令
    update.go               事件处理（按键、防抖到期、异步回包）
    view.go                 布局渲染（头部/列表/预览/状态栏）
    styles.go               Lipgloss 样式
testdata/demo.png           图片预览测试素材
```

## 已知限制 / Roadmap

- IDE 运行窗/管道等无 TTY 环境会自动降级为单帧渲染（提示后以 `--once` 效果输出），交互模式请在真实终端运行
- 图片协议探测靠环境变量启发式；sixel 精确探测需 DA1 查询（待实现）
- 预览区滚动大图时图形序列可能被 viewport 切分（待验证/优化）
- 文件模式为子串匹配，后续可换模糊匹配 + 排序评分
- 列表无虚拟化（当前上限 500 条，够用）
- 长行截断而非折行（保持 1 行号 = 1 源行，跳转行计算简单）
