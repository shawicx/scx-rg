# logs —— docker / k8s 日志源

包路径：`internal/logs`。为 `scx-rg docker` / `scx-rg k8s` 子命令与 `--provider docker-ps` 提供数据。

## 结构

| 类型/函数 | 文件 | 说明 |
| --- | --- | --- |
| `Target{Kind, Name, Namespace, Container}` | docker.go | 一次抓取目标；`Bin()` 返回 docker/kubectl，`Available()` 用 LookPath 探测 |
| `Source{Target, Detail, Status}` | sources.go | 列表项（Detail=镜像/namespace，Status=Up x days / Running） |
| `ListSources(ctx, run, kind)` | sources.go | docker 跑 `ps -a --format json`（JSON 行逐行 Unmarshal，坏行跳过）；kubectl 跑 `get pods -o json`；`run Runner` 可注入 fake（测试） |
| `Snapshot(ctx, run, t, tail)` | docker.go | `docker logs --tail N -t` / `kubectl logs --timestamps --tail` 一次性落盘（`--snapshot` 旧流程用） |
| `Stream(ctx, t, tail, path, onLine)` | stream.go | `logs -f --timestamps --tail` 长驻进程：stdout 与 stderr 合流到同一管道逐行读，每行 tee 写 path（O_TRUNC 起笔）并回调 onLine；err==nil ⇔ 干净结束（容器停止/ctx 取消都算），非零退出且非取消才报错 |
| `LivePath(base, t)` | stream.go | 实时 tee 落盘的稳定路径：`base/<kind>/[<ns>/]<name>.log`（kubectl 按 namespace 分目录，未指定用 default）——「默认 scx-rg 命令搜日志」成立的前提 |

抓取行数上限：main 的 `logTail = 100000`（`--tail` 直传底层命令）。stderr 合流的原因：docker CLI 对容器两个输出流分流（容器 stderr 走 CLI 自身 stderr），只接 stdout 会丢纯 stderr 日志；启动错误文本（如「No such container」）也随流入面板，无需单独缓冲。

## 实时会话流程（tui/picker.go + live.go 配合）

```text
scx-rg docker（无名字）
  → 阶段1 picker：loadPicker → ListSources → pickerFilter（逐键模糊过滤，无防抖）
      Tab 多选标记（≤4，第 5 个提示上限）→ Enter 用标记集（无标记用光标项）
  → 阶段2 实时态（live.go）：startLive([]Target) 每目标一个 Stream 进程
      + tee 落盘 LivePath(LiveDir)，面板缓冲环形窗口 5000 行
      Ctrl+R → reenterPicker 回阶段1（停全部流进程、清实时态、重载源列表）
  → 会话退出后落盘文件保留，默认 scx-rg 命令 / --follow 随时可搜

scx-rg docker <名字>   LiveTargets 直达实时单面板（stderr 打印落盘路径）
```

docker/k8s 实时模式强制 `ImgProto=ProtocolNone`（日志面板不预览图片），且不依赖 rg。`Ctrl+R` 一键双义：picker 阶段=刷新列表（`handlePickerKey`），实时阶段=返回选择器（`handleLiveKey`）。

## --snapshot 旧流程（兼容保留）

`scx-rg docker --snapshot`：runSnapshotSession 抓一次 Snapshot 到临时目录（退出自动清理），进入既有检索态（Root=临时目录 + ModeContent + PickLine，Enter 输出选中日志行文本）；选择器内 Tab 多选禁用（`LivePick=false`）。实时/快照以外的旧 `Follow`/`FollowPick` 路径已删除。

## 文件跟随（tui/follow.go）

`Config.FollowFile` 非空时（顶层 `scx-rg --follow <文件>`：本地日志或实时会话的落盘文件）：`Init` 起 `followTick` 链，每 800ms stat 文件，**只在增长时**重跑当前查询；用 `followKeep`（path:line，即 `resultKey`）在刷新后恢复选中位置。与实时视图（live.go）互不相关——前者是搜索界面的刷新语义，后者是分屏贴底语义。

## 与 finder 的关系（M4）

`--provider docker-ps` 复用 `ListSources`，但只取容器名作候选（Detail 拼镜像·状态），单阶段直接输出选中名字——与 docker 子命令（选择器+实时多面板）互补：前者适合 `docker stop $(scx-rg --provider docker-ps)`，后者是看日志。

Related: [tui（picker/实时面板/跟随状态）](tui.md) · [interaction（子命令用法）](../03-guides/interaction.md)
