# scx-rg

终端里的实时搜索 + 预览工具。Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss)。

## 安装

从 [Releases](https://github.com/shawricx/scx-rg/releases) 下载对应平台的 `scx-rg_<版本>_<os>_<arch>.tar.gz`（macOS / Linux × amd64 / arm64）：

```bash
tar -xzf scx-rg_0.1.0_darwin_arm64.tar.gz
sudo mv scx-rg /usr/local/bin/
scx-rg --version    # scx-rg 0.1.0 (commit …, built …, darwin/arm64)
```

建议先核对校验和（与压缩包同目录的 `scx-rg_<版本>_checksums.txt`）：

```bash
shasum -a 256 --check scx-rg_0.1.0_checksums.txt --ignore-missing
```

macOS 首次运行未签名二进制若被 Gatekeeper 拦截：`xattr -d com.apple.quarantine /usr/local/bin/scx-rg`。

## 功能

- **实时防抖搜索**：输入即搜（默认 200ms 防抖），过期结果自动丢弃（版本号判废）
- **双模式**：`Tab` 切换
  - 文件模式：fzf 式模糊匹配（空格分词 AND、边界/连续加权评分排序，命中字符高亮）；完整子串命中大幅优先，散落拼凑的低质量匹配直接过滤（宁缺毋滥）；`Ctrl+F` 可切换为精确匹配（分词必须是完整子串）；优先用 `rg --files` 枚举（尊重 .gitignore），无 rg 时回退内置遍历
  - 内容模式：`rg --json` 流式解析，结果边搜边出；输入变化立即杀掉上一轮 rg 进程；查询默认按正则解析，**不是合法正则时自动按字面量兜底**（搜 `log.error(` 这类含元字符的文本不报错，状态栏提示），`Ctrl+F` 可手动粘性切换字面量（-F）/正则
- **多面板预览**：左侧结果列表、右侧预览面板
  - 代码：chroma 语法高亮 + 行号槽，内容模式自动跳转到匹配行
  - 图片：三档渲染——kitty 图形协议 / sixel 协议直接在终端内显示，均不可用时降级 halfblock（`▀` 半块字符 + truecolor/256 色，任何彩色终端可用）；GIF 显示首帧
- **fzf 式工作流**：`Enter` 退出并把选中文件绝对路径打印到 stdout，可接管道

## Docker / Kubernetes / 服务器日志检索

```bash
scx-rg docker                        # 交互选择容器（模糊过滤，免记名字），默认实时跟随
scx-rg docker <容器名>                # 直达：tail 最近 100000 行并实时跟随
scx-rg docker <容器名> --snapshot     # 只抓一次快照（不跟随）

scx-rg k8s                           # 交互选择 Pod，默认实时跟随
scx-rg k8s <Pod名> [-n namespace] [-c 容器] [--snapshot]

scx-rg --follow /var/log/app.log     # 本地服务器日志实时跟随
```

- **默认跟随**：日志是活数据，`docker logs -f --tail` 的初始内容与快照完全相同，且此后实时更新——不会再出现「刚产生的日志搜不到」；`--snapshot` 退回一次性快照
- **最新优先**：命中数很多时（如搜 `INFO`），日志模式保留**最新的 5000 条命中**（旧的滚出），配合 Ctrl+T 条数/时间筛选进一步收窄

### 容器 / Pod 选择器

`scx-rg docker` / `scx-rg k8s` 不带名字时进入选择器：

- 左侧列出全部容器（名称 · 镜像 · 状态，`Up`/`Running` 绿色标注）或 Pod（名称 · namespace · `就绪数/总数 状态`），右侧显示选中目标详情
- 输入即模糊过滤（名称+镜像/namespace），`↑↓` 选择，`Ctrl+R` 刷新列表
- `Enter` 抓取该目标最近 100000 行日志，无缝切入全文检索界面（配合过 `--follow` 则改为实时跟随）
- 抓取失败（如 daemon 未启动）会显示错误并停留在选择器，可重试或换目标

- 跟随模式：日志持续写入快照，界面每 800ms 检测增长并自动重跑当前查询；**保持你的选中位置**（path:line 对齐），状态栏显示「* 跟随 / 大小」
- **Ctrl+T 可视化筛选**：「过去 15 分钟」「最近 100 条」等常用参数直接在界面上选，即时生效，详见下方按键表
- 快照（`docker logs` / `kubectl logs --timestamps --tail`）带时间戳；`Enter` 把选中的日志行文本输出到 stdout；快照文件退出自动清理
- **大日志窗口化预览**：超过 1MB 的文件不再拒绝预览，只渲染命中行前后 40/80 行的上下文窗口（真实行号 + `...` 跳过标记 + 长行折行），搜索命中自动定位
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

### Linux 交叉编译（部署服务器）

在 macOS 上直接产出 Linux 静态二进制（CGO 关闭，零依赖，服务器不需要装 Go）：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o scx-rg-linux .

# 校验后传到服务器（传完先核对 sha256，排除传输损坏）
shasum -a 256 scx-rg-linux
scp scx-rg-linux user@server:/usr/local/bin/scx-rg

# ARM 服务器（如树莓派/国产 ARM 云主机）改 GOARCH 即可
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o scx-rg-linux .
```

服务器上只需要 [ripgrep](https://github.com/BurntSushi/ripgrep)（`apt install ripgrep` 或 `yum install ripgrep`）。

## 按键

| 按键 | 作用 |
| --- | --- |
| 输入 | 实时搜索（防抖） |
| `↑` `↓` / `Ctrl+P` `Ctrl+N` | 选择结果 |
| `Tab` | 切换 文件/内容 模式 |
| `Ctrl+F` | 匹配行为切换：文件模式=精确（子串）/ 模糊；内容与全文回退=字面量（-F）/ 正则 |
| `Ctrl+T` | 打开可视化筛选栏（时间范围 / 条数封顶） |
| `Ctrl+O` | 在 less 中打开当前预览文件（自由选择复制文本，`q` 返回） |
| `Ctrl+Y` | 复制选中行（日志模式）/ 绝对路径（文件模式）到剪贴板 |
| `PgUp` `PgDown` | 滚动预览 |
| `Enter` | 选定（stdout 输出路径）并退出 |
| `Esc` | 清空搜索词；已空则退出 |
| `Ctrl+C` | 退出 |

### 复制文本

全屏 TUI 里终端原生选择会把边框和双面板一起框住，没法只选预览里的文本。两个出口：

- **`Ctrl+O` 翻页器**：临时释放终端，在 less 中打开当前预览的文件（自动定位到选中行）——纯文本环境里随意滚动、选择、复制，还能用 less 自带的 `/` 搜索，`q` 返回 TUI
- **`Ctrl+Y` 剪贴板**：走 OSC 52 转义序列，SSH 远程会话同样有效。iTerm2 需开启 `Applications in terminal may access clipboard`；tmux 需 `set -g set-clipboard on`

### 可视化筛选栏（Ctrl+T）

```
时间   实时  全部  1分钟  5分钟  15分钟  1小时  6小时  24小时
条数   全部  100条  500条  5000条
```

- `←` `→` 在预设间移动光标，**即时生效**；`↑` `↓`/`Tab` 切换两段；`Enter`/`Esc` 收起
- **实时**：滑动窗口，只看最近 30 秒；配合 `--follow` 即实时日志窗——即使没有新日志写入，超窗的旧行也会随时间自动滚出列表，窗口每秒滑动一次
- **时间**：只显示行首时间戳在「过去 X 分钟/小时」内的行（自动识别 docker/kubectl 快照、`2026-08-20 10:00:00`、nginx、syslog 等常见格式）；无时间戳的行（多行堆栈续行）保留；检测不到时间戳时该段失效并提示
- **条数**：只保留最新的 N 条命中（配合搜索词就是「最近 100 条 ERROR」）
- 全部在客户端完成，不重新抓取日志；跟随模式（--follow）下每次刷新自动重新过滤，实时/时间窗随时间滑动；静态快照只按抓取时刻过滤一次（不滑动）
- 生效的筛选显示在底部状态栏，如 `实时 / 末100条`

## 参数

```
-path string    搜索根目录（默认 .）
-mode string    files | content（默认 files）
-img string     auto | kitty | sixel | halfblock | none（默认 auto，自动探测：环境变量 → DA1 查询 → halfblock 兜底）
-debounce-ms    搜索防抖间隔（默认 200）
-version        输出版本信息并退出
-once           渲染一帧后退出（调试/CI 冒烟）
-q string       配合 --once 模拟搜索词
-preview-file   配合 --once 强制预览指定文件
-w / -h         配合 --once 的渲染尺寸
```

## 图片预览协议

自动探测按优先级选择渲染档位：

1. **kitty 图形协议**：`KITTY_WINDOW_ID` / `TERM` 含 kitty / ghostty / `WEZTERM_PANE` 等环境标志（kitty 协议没有 DA1 标志位，环境变量是唯一可靠依据）
2. **sixel**：启动时向控制终端发 DA1 查询（`ESC[c`，raw mode + 150ms 超时），响应属性含 `4` 即支持——覆盖 xterm `-ti vt340`、alacritty-sixel、st 等无环境标志的终端；查询无响应时回退 `TERM` 启发式（foot / yaft / mlterm）
3. **halfblock**：`▀` 半块字符 + 前景/背景双色（每字符格承载上下两个像素行），按终端能力自动降级 truecolor → 256 色 → 16 色，任何彩色终端可用

`--img none` 显式禁用（显示文件信息占位盒）；无色彩能力的输出（管道 / CI）同样回退占位盒。

### 真机实测 checklist（M3）

代码侧防残留已内建并有测试覆盖，以下需在真终端人工确认（素材 `testdata/demo.png`）：

- [ ] kitty / ghostty / wezterm：`scx-rg testdata`（或 `--img kitty`）选中 demo.png，图片在预览面板内正确显示
- [ ] 上下切换「图片 ↔ 代码 ↔ 图片」：旧图立即消失，无残影、不错位
- [ ] 窗口 resize：图片随面板重绘，尺寸跟随
- [ ] `Esc` 退出 / `Enter` 选定退出后：终端内无图像残留（退出时会主动发 kitty 图形清除序列）
- [ ] sixel 终端（foot / xterm -ti vt340 / wezterm `--img sixel`）：同上切换与退出场景
- [ ] 普通终端（iTerm2 / Apple Terminal / VSCode）：自动降级 halfblock，`▀` 半块图色彩正确、边框对齐

## 发版（维护者）

版本号以 git tag（`vX.Y.Z`）为唯一来源，发版不需要改任何代码：

```bash
git tag v0.1.0
git push origin v0.1.0
```

推送 tag 后 GitHub Actions 自动跑测试、交叉编译 macOS/Linux × amd64/arm64 四个平台的压缩包，并创建 GitHub Release（附 sha256 校验和）。Release 正文由 [git-cliff](https://git-cliff.org) 按 conventional commit 前缀自动生成分组变更记录（`🚀 新功能` / `🐛 Bug 修复` 等，配置见 `cliff.toml`）。本地 `go build` 出来的二进制 `--version` 显示 `dev`，正式版本号在 CI 构建时注入。

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
    image.go                kitty/sixel 图形协议编码 + 图形删除清理
    halfblock.go            半块字符 + truecolor/256 色的第三档渲染
    cellsize_unix.go        TIOCGWINSZ 取单元格像素尺寸
    protocol.go             图形协议探测（环境变量 → DA1 → halfblock）
    da1_unix.go             DA1 查询（raw mode + 超时读，探测 sixel）
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
- 图片渲染防残留机制已内建（换图前删旧 placement、切走时注入删除序列、退出时清空全部图形、图片预览禁滚动）；kitty overlay 与文本滚动的完全同步需独立渲染层，列入 backlog
- SSH 远程使用时环境变量不透传，本地 kitty 可能被探测为 halfblock——可 `--img kitty` 强制指定（或在 SSH 配置 `SetEnv KITTY_WINDOW_ID`）
- 「歧义宽=2」终端（如开启 iTerm2 ambiguous double 选项）中 halfblock 的 `▀` 字符会按 2 格渲染导致错位，可用 `--img none` 关闭
- GIF 只显示首帧（动画播放列入 backlog）
- 文件模式为子串匹配，后续可换模糊匹配 + 排序评分
- 列表无虚拟化（当前上限 500 条，够用）
- 预览长行按面板宽度折行显示（行号只在首段），单行最多折 10 段、超出以 ... 标记（防超长 JSON 行撑爆视口）；界面字符与折行宽度按「歧义宽字符=2 格」保守计算，避免中文终端（如开启 iTerm2 歧义宽选项）整帧错位
