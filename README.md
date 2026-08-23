# scx-rg

终端里的实时搜索 + 预览工具。Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss)。

## 安装

一键安装（自动识别平台，下载最新 Release 并校验 sha256，装到 /usr/local/bin）：

```bash
curl -fsSL https://raw.githubusercontent.com/shawricx/scx-rg/main/scripts/install.sh | sh
# 或装到自定义目录
curl -fsSL https://raw.githubusercontent.com/shawricx/scx-rg/main/scripts/install.sh | sh -s -- --bin ~/.local/bin
```

或手动从 [Releases](https://github.com/shawricx/scx-rg/releases) 下载对应平台的 `scx-rg_<版本>_<os>_<arch>.tar.gz`（macOS / Linux × amd64 / arm64）：

```bash
tar -xzf scx-rg_0.0.1_darwin_arm64.tar.gz
sudo mv scx-rg /usr/local/bin/
scx-rg --version    # scx-rg 0.0.1 (commit …, built …, darwin/arm64)
```

建议先核对校验和（与压缩包同目录的 `scx-rg_<版本>_checksums.txt`）：

```bash
shasum -a 256 --check scx-rg_0.0.1_checksums.txt --ignore-missing
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
- **多选输出**：`Ctrl+Space` 标记/取消当前行（自动下移），`Enter` 一次输出全部标记项（多行）；`Esc` 递进清空（输入 → 标记 → 退出）
- **帮助浮层**：输入为空时按 `?`（或 `F1`）查看按当前模式裁剪的完整键位表，任意键返回
- **通用 finder**：`--provider stdin` 读管道候选行做模糊筛选（`Enter` 输出原行文本）；`--provider docker-ps` 内置容器列表；候选若恰是文件路径则自动获得正常预览
- **编辑器集成**：配置 `[editor]` 后 `Ctrl+E` 把选中文件在编辑器中打开到对应行（`{file}`/`{line}` 模板，预置 nvim/vim/code/emacs/zed 参数），编辑器退出自动返回 TUI；Enter 的 stdout 输出契约不变
- **命令面板**：输入为空时按 `:` 打开，模糊过滤命令（模式切换/筛选栏/主题循环/帮助/退出），`↑↓` 选择 `Enter` 执行
- **Git 筛选**：筛选栏（Ctrl+T）在 git 仓库内多出第三段「全部 / 仅改动 / 仅暂存」，文件模式在枚举层过滤、内容模式按路径过滤；未跟踪新文件不在 git diff 语义内
- **命名主题**：`[theme] preset = default | dracula | nord | catppuccin`，也可在命令面板循环切换（会话级）；显式 hex 三色仍可覆盖 preset
- **日志级别高亮**：预览正文内 ERROR/FATAL/PANIC（红）、WARN（黄）、INFO/DEBUG（暗）按词边界着色，与语法色/命中高亮叠加互不破坏
- **配置文件**：`~/.config/scx-rg/config.toml` 自定义防抖、忽略目录、主题与编辑器，见下文

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
| `Ctrl+T` | 打开可视化筛选栏（时间范围 / 条数封顶 / git 仓库内含「仅改动/仅暂存」） |
| `Ctrl+E` | 在编辑器打开选中文件到对应行（需配置 `[editor]`） |
| `:` | 命令面板（输入为空时） |
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
-provider string stdin | docker-ps（管道/容器候选取代文件搜索，Enter 输出原行文本）
-debounce-ms    搜索防抖间隔（默认 200，可被 config.toml 覆盖）
-version        输出版本信息并退出
-once           渲染一帧后退出（调试/CI 冒烟）
-q string       配合 --once 模拟搜索词
-preview-file   配合 --once 强制预览指定文件
-w / -h         配合 --once 的渲染尺寸
```

## 通用 finder（--provider）

把任意「一行一候选」的输出交给 scx-rg 做模糊筛选，`Enter` 输出选中行文本（支持 `Ctrl+Space` 多选）：

```bash
fd --type f | scx-rg --provider stdin        # 文件选择（候选是真实路径时自动获得预览）
git branch | scx-rg --provider stdin         # 任意列表
scx-rg --provider docker-ps                  # 内置：docker 容器（镜像 · 状态做详情）
docker stop $(scx-rg --provider docker-ps)   # 组合用法
```

## 配置文件（~/.config/scx-rg/config.toml）

未配置时全部使用内置默认；配置损坏回退默认并警告，不阻断启动。优先级：flag 显式设置 > config.toml > 默认。

```toml
# 搜索防抖间隔（毫秒）
debounce_ms = 200

# 额外忽略的目录名（追加到内置忽略；对 rg 枚举与内置遍历都生效）
ignore = ["build", ".venv"]

[theme]
# 命名主题：default | dracula | nord | catppuccin（空 = default；
# 也可在命令面板 : 循环切换，会话级）
preset = "default"
# 显式 hex 三色，优先于 preset 同槽位
accent    = "#7D56F4"  # 标题底色 / 激活边框 / 选中行
match     = "#56C9F4"  # 命中高亮 / 输入提示符
row_marker = "#3DDC97" # 行标记 > ✓

[editor]
# Ctrl+E 打开选中文件到对应行；command 留空 = 键位隐藏。
# args 支持 {file}（绝对路径）与 {line} 模板变量；留空时按命令名套用
# nvim/vim/emacs（+行号 文件）、code（--goto 文件:行）、zed（文件:行）预置。
command = "nvim"
args = ["+{line}", "{file}"]
```

## shell 集成（CTRL-T / CTRL-R）

[examples/scx-rg.zsh](examples/scx-rg.zsh) 与 [examples/scx-rg.fish](examples/scx-rg.fish) 提供 fzf 式键绑定：`CTRL-T` 从 `fd` 文件列表选文件插入命令行，`CTRL-R` 模糊搜索命令历史（zsh 用 `fc -l`、fish 用 `builtin history` 喂给 `--provider stdin`，无需内置历史源）：

```zsh
# ~/.zshrc
source /path/to/scx-rg/examples/scx-rg.zsh
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
main.go                     入口：参数解析、配置加载、协议探测、provider 分发、程序启动
internal/
  config/
    config.go               ~/.config/scx-rg/config.toml 读取（防抖/忽略/主题）
  search/
    provider.go             Provider / SyncProvider / StreamProvider 接口
    fuzzy.go                模糊匹配与评分（子序列 + 边界/连续加权）
    files.go                文件名搜索：rg --files / 内置遍历 + 模糊排序
    list.go                 静态候选搜索（--provider stdin / docker-ps）
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
    help.go                 帮助浮层（? / F1，按模式裁剪的键位表）
    styles.go               Lipgloss 样式 + ApplyTheme 主题注入
testdata/demo.png           图片预览测试素材
examples/                   zsh/fish 集成示例（CTRL-T 文件 / CTRL-R 历史）
```

## 测试

```bash
go test ./...
```

- `internal/search`：模糊匹配评分/分词语义、rg --files 的 gitignore 行为、**rg --json 事件解析纯单测**（`parseRgLine`，不依赖真实 rg、CI 恒跑）、流式搜索与取消不泄漏、walk 忽略规则（skipDirs + 隐藏目录）
- `internal/preview`：高亮行数对齐、CJK/emoji 折行、超长单行段数封顶（maxWrapSegments）、二进制嗅探、大文件窗口化、图片三档渲染与协议探测
- `internal/tui`：流式结果追加、过期消息丢弃、新搜索重置状态（通过 drain 驱动 cmd 链模拟事件循环）、多选/帮助/finder/清理注入
- **golden frame 快照**（`internal/tui/golden_test.go`）：`RenderOnce`/`View` 整帧去 ANSI 后与 `internal/tui/testdata/golden/*.txt` 逐字节对比（files 过滤、finder 详情、帮助浮层、多选标记、图片占位、Git 筛选栏、命令面板七个场景，全部 rg-free 确定性渲染）。有意改动界面后刷新基线：

```bash
go test ./internal/tui -update   # 重新生成 golden 基线，人工审查 diff 后提交
```

## 已知限制 / Roadmap

- IDE 运行窗/管道等无 TTY 环境会自动降级为单帧渲染（提示后以 `--once` 效果输出），交互模式请在真实终端运行
- 图片渲染防残留机制已内建（换图前删旧 placement、切走时注入删除序列、退出时清空全部图形、图片预览禁滚动）；kitty overlay 与文本滚动的完全同步需独立渲染层，列入 backlog
- SSH 远程使用时环境变量不透传，本地 kitty 可能被探测为 halfblock——可 `--img kitty` 强制指定（或在 SSH 配置 `SetEnv KITTY_WINDOW_ID`）
- 「歧义宽=2」终端（如开启 iTerm2 ambiguous double 选项）中 halfblock 的 `▀` 字符会按 2 格渲染导致错位，可用 `--img none` 关闭
- GIF 只显示首帧（动画播放列入 backlog）
- 文件模式为子串匹配，后续可换模糊匹配 + 排序评分
- 列表无虚拟化（当前上限 500 条，够用）
- 预览长行按面板宽度折行显示（行号只在首段），单行最多折 10 段、超出以 ... 标记（防超长 JSON 行撑爆视口）；界面字符与折行宽度按「歧义宽字符=2 格」保守计算，避免中文终端（如开启 iTerm2 歧义宽选项）整帧错位
