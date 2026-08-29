---
title: 日志(Docker / K8s 实时 + 搜索)
description: 实时多面板看日志,默认命令搜落盘文件,--snapshot 保留旧快照检索。
---

实时看日志与搜索日志是两个独立入口:`docker`/`k8s` 子命令只做实时多面板(lazydocker 式),边渲染边 tee 落盘;默认 `scx-rg` 命令随时检索落盘文件。

## 命令矩阵

```bash
scx-rg docker                        # 选择器:Tab 多选 ≤4,Enter 进实时分屏
scx-rg docker <容器名>                # 直达:跳过选择器,实时全屏单面板
scx-rg docker <容器名> --snapshot     # 旧流程:一次性快照 + 检索界面

scx-rg k8s                           # 选择器(Pod,Tab 多选 ≤4)
scx-rg k8s <Pod名> [-n namespace] [-c 容器] [--snapshot]

scx-rg <落盘文件>                     # 默认命令检索实时会话留下的日志
scx-rg --follow <落盘文件>            # 边跟边搜(实时会话还在跑时效果最佳)
scx-rg --follow /var/log/app.log     # 本地服务器日志实时跟随(不受影响)
```

子命令上的 `--follow` / `-f` 旗标兼容保留(实时已是默认行为,加了无差别);实时视图本身不依赖 ripgrep,`--snapshot` 检索需要。

## 实时多面板

| 功能 | 说明 | 操作方式 |
| --- | --- | --- |
| 多容器实时日志 | 最多 4 个容器分屏(1 全屏 / 2 上下 / 3 上 1 下 2 / 4 田字),流式追加自动贴底;面板标题 `●` 流存活 / `■` 容器已停止(缓冲保留可翻阅,不自动重启),某目标启动失败只在该面板内显示错误 | `scx-rg docker` 选择器 Tab 多选后 `Enter`;`scx-rg docker <名>` 直达全屏 |
| 焦点面板滚动 | 上翻即暂停**该面板**跟随(其余面板不受影响),回底自动恢复 | `j` `k` `↑` `↓`、`Ctrl+D`/`Ctrl+U`、`PgUp`/`PgDn`;`G`/`End` 回底恢复,`g`/`Home` 到顶 |
| 焦点切换 | 焦点面板激活边框 + 标题 `◀` 指示 | `Tab`/`Shift+Tab` 循环;`1`-`4` 直达 |
| 复制搜索命令 | 把焦点面板落盘文件拼成 `scx-rg --follow <路径>` 复制到剪贴板(OSC 52,另开终端粘贴即边跟边搜) | 实时视图按 `y` |
| tee 落盘 | 实时日志同步写入 `<UserCacheDir>/scx-rg/logs/<kind>/[<ns>/]<名>.log`(macOS `~/Library/Caches/scx-rg/logs/…`,Linux `~/.cache/scx-rg/logs/…`;k8s 按 namespace 分目录);带时间戳;会话启动时重写、退出后保留 | 自动进行,状态栏显示落盘目录 |
| 帮助 / 重选 / 退出 | 实时键位浮层;停掉全部流进程回选择器换目标;清理退出 | `?`/`F1`;`Ctrl+R`/`Alt+R`;`Ctrl+C`/`Esc` |

## 容器 / Pod 选择器

`scx-rg docker` / `scx-rg k8s` 不带名字时进入选择器:

- 左侧列出全部容器(名称 · 镜像 · 状态,`Up`/`Running` 绿色标注)或 Pod(名称 · namespace · `就绪数/总数 状态`),右侧显示选中目标详情
- 输入即模糊过滤(名称+镜像/namespace),`↑↓` 选择,`Ctrl+R` 刷新列表
- `Tab` 标记/取消标记目标(最多 4 个,标第 5 个提示上限);`Enter` 进实时分屏——有标记用标记集,无标记用光标项
- 抓取失败(如 daemon 未启动)显示错误并停留,可重试或换目标
- **反悔随时回去**:实时阶段再按 `Ctrl+R`(或 `Alt+R`)返回选择器重新选目标——停掉全部实时进程,列表会重新加载,容器可能已增减

## 搜索日志(默认 scx-rg 命令)

实时会话的落盘文件是普通文件(且路径稳定可预测),默认命令全功能可用:

```bash
scx-rg ~/Library/Caches/scx-rg/logs/docker/web.log           # 检索落盘日志(留空即全量)
scx-rg --follow ~/Library/Caches/scx-rg/logs/docker/web.log  # 边跟边搜
```

- **边跟边搜**:`--follow` 下界面每 800ms 检测文件增长并自动重跑当前查询,**保持你的选中位置**,状态栏显示「* 跟随 / 大小」
- **留空即全量**:不输入关键词直接展示抓到的全部日志,输入后实时过滤,清空回到全量
- **最新优先**:命中数很多时保留**最新的 5000 条**(旧的滚出),配合 [Ctrl+T 筛选](/guides/filtering/)进一步收窄
- 落盘日志带时间戳(docker/kubectl `--timestamps`),时间筛选可用
- `Enter` 把选中的日志行文本输出到 stdout;超 1MB 的大日志照样预览(命中行前后 40/80 行上下文,真实行号 + `...` 标记)

预览正文中日志级别自动着色:ERROR/FATAL/PANIC 红、WARN 黄、INFO/DEBUG 暗。

## 快照检索(--snapshot,兼容旧流程)

`scx-rg docker --snapshot <名>` 走分离前的旧路径:一次性抓最近 100000 行快照到临时目录并进入既有检索界面(无跟随),快照文件退出自动清理;选择器内多选禁用,`Enter` 即单目标快照检索。
