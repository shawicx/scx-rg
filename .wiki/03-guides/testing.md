# 测试体系

```bash
go test ./...                          # 全量（CI 同款）
go test ./internal/tui -update         # 刷新 golden frame 基线（人工审查 diff 后提交）
```

全部 stdlib `testing`，零断言库依赖。

## 各包覆盖

| 包 | 重点 |
| --- | --- |
| `internal/search` | 模糊评分/分词语义、rg --files gitignore 行为、walk 忽略规则（skipDirs + 隐藏目录）、**rg --json 解析纯单测**（`parseRgLine`，CI 无 rg 恒跑）、流式取消不泄漏、exit 2 容错、ListProvider |
| `internal/preview` | 高亮行数对齐（highlightLines 长度=Split）、CJK/emoji 折行宽度、超长单行段数封顶、二进制嗅探、大文件窗口化 + JumpOffset、图片三档渲染序列、协议探测链、halfblock 各色彩档位、GIF 首帧还原、缓存 |
| `internal/tui` | 见下（harness + golden） |
| `internal/config` | 字段解析、缺文件/坏文件回退默认、零值保持默认 |
| `internal/logs` | docker/kubectl 输出解析（Runner 注入 fake）、`Stream` 流式读取与 tee 落盘（streamLoop 纯单测喂 strings.NewReader，streamCommand 跑真实 `sh -c` 子进程验 stderr 合流/非零退出；ctx 取消契约在 tui 侧 `TestLiveReenterPickerStopsStreams`）、`LivePath` 路径拼装 |

## tui 测试 harness：drain

`tui.Model.drain(cmd)`（生产代码）同步驱动 cmd/msg 链：执行 cmd 得 msg → `m.Update(msg)` → 收集 next cmd 循环；展开 `tea.BatchMsg`、丢弃 `cursor.BlinkMsg`（自续链否则死循环）、上限 2^20。配合注入点：

- `m.renderFile`——换 fake 渲染函数（计数/注入 kitty 序列）
- `m.writeClipboard`、`m.now`（假时钟）、`Config.ListSources/FetchLog/StreamLog`（实时流注入 fakeStream：逐行直接回调后收束，无真实 docker 依赖，见 live_test.go）
- `newContentModel`（update_test.go）——rg 依赖场景的模型构造器，**rg 未装自动 skip**

rg 依赖测试在 CI（无 rg）会 skip 是预期行为；rg 解析逻辑由 `rg_parse_test.go` 纯单测兜底，CI 恒跑。

## golden frame 快照（M5）

`internal/tui/golden_test.go` + `live_test.go`：十三个确定性场景——files 过滤、finder 详情、帮助浮层、多选标记、图片占位、Git 筛选栏、命令面板、搜索历史、blame 状态栏、AST 替换浮层、实时分屏 1/2/4 面板（live-1/live-2/live-4，`StreamLog` 注入 fake 流）——整帧经 `RenderOnce`/`View` 渲染，**去 ANSI**（正则剥 CSI/OSC/kitty APC）后与 `internal/tui/testdata/golden/*.txt` 逐字节对比。

确定性设计：场景全部 rg-free（`RgAvailable:false` 走 walkFiles 排序）、非空 query 有排序契约、相对路径显示、无时间戳——本机与 CI 输出一致。

失败时输出首差异行 ±3 行上下文。**有意改界面后**：`go test ./internal/tui -update` 刷新 → 人工审查 diff → 提交。

历史战绩：golden 首次落地即抓出两个真 bug（状态栏提示超宽折行破坏帧高不变式；帮助浮层窄终端右列被整列截掉）。

## 帧不变式回归（frame_width_test.go）

「帧行数恒等于终端高度、行宽不超 frameW」——针对中文环境歧义宽终端的整帧错位问题（见 [design-decisions](../04-decisions/design-decisions.md)）。改动布局/样式后必跑。

Related: [tui](../02-modules/tui.md) · [release（CI 怎么跑测试）](release.md)
