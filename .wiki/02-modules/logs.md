# logs —— docker / k8s 日志源

包路径：`internal/logs`。为 `scx-rg docker` / `scx-rg k8s` 子命令与 `--provider docker-ps` 提供数据。

## 结构

| 类型/函数 | 文件 | 说明 |
| --- | --- | --- |
| `Target{Kind, Name, Namespace, Container}` | docker.go | 一次抓取目标；`Bin()` 返回 docker/kubectl，`Available()` 用 LookPath 探测 |
| `Source{Target, Detail, Status}` | sources.go | 列表项（Detail=镜像/namespace，Status=Up x days / Running） |
| `ListSources(ctx, run, kind)` | sources.go | docker 跑 `ps -a --format json`（JSON 行逐行 Unmarshal，坏行跳过）；kubectl 跑 `get pods -o json`；`run Runner` 可注入 fake（测试） |
| `Snapshot(ctx, run, t, tail)` | docker.go | `docker logs --tail N -t` / `kubectl logs --timestamps --tail` 一次性落盘 |
| `Follow(ctx, t, tail, path)` | docker.go | `-f --tail N` 持续追加到快照文件 |

抓取行数上限：main 的 `logTail = 100000`（`--tail` 直传底层命令）。

## 两阶段流程（tui/picker.go 配合）

```text
scx-rg docker（无名字）
  → 阶段1 picker：loadPicker → ListSources → pickerFilter（逐键模糊过滤，无防抖）
      Enter → fetchTarget 抓日志落临时目录（SnapshotDir，退出自动清理）
  → 阶段2 检索态：Root=临时目录 + ModeContent + FollowFile(跟随模式) + PickLine
      （Enter 输出选中日志行文本而非路径；日志文件路径对用户无意义）
      Ctrl+R → reenterPicker 回到阶段1（停搜索与跟随进程、清检索态、重载源列表）

scx-rg docker <名字>   跳过阶段1直达（stderr 提示后先抓再进 TUI）
```

docker/k8s 模式强制 `ImgProto=ProtocolNone`（临时日志不预览图片）。`Ctrl+R` 一键双义：picker 阶段=刷新列表（`handlePickerKey`），检索阶段=返回选择器（`handleKey` 判 `pickerKind != ""`）。

## 跟随模式（tui/follow.go）

`Config.FollowFile` 非空时：`Init` 起 `followTick` 链，每 800ms stat 文件，**只在增长时**重跑当前查询；用 `followKeep`（path:line，即 `resultKey`）在刷新后恢复选中位置。`--follow /var/log/app.log` 本地文件同机制（tail -f 式）。

## 与 finder 的关系（M4）

`--provider docker-ps` 复用 `ListSources`，但只取容器名作候选（Detail 拼镜像·状态），单阶段直接输出选中名字——与 docker 子命令（两阶段+日志检索）互补：前者适合 `docker stop $(scx-rg --provider docker-ps)`，后者是日志检索。

Related: [tui（picker/跟随状态）](tui.md) · [interaction（子命令用法）](../03-guides/interaction.md)
