---
title: 日志检索(Docker / K8s / 跟随)
description: 交互选容器、实时跟随、换目标,本地日志 tail -f。
---

## 三种入口

```bash
scx-rg docker                        # 交互选择容器(模糊过滤,免记名字),默认实时跟随
scx-rg docker <容器名>                # 直达:tail 最近 100000 行并实时跟随
scx-rg docker <容器名> --snapshot     # 只抓一次快照(不跟随)

scx-rg k8s                           # 交互选择 Pod
scx-rg k8s <Pod名> [-n namespace] [-c 容器] [--snapshot]

scx-rg --follow /var/log/app.log     # 本地服务器日志实时跟随
```

## 容器 / Pod 选择器

`scx-rg docker` / `scx-rg k8s` 不带名字时进入选择器:

- 左侧列出全部容器(名称 · 镜像 · 状态,`Up`/`Running` 绿色标注)或 Pod(名称 · namespace · `就绪数/总数 状态`),右侧显示选中目标详情
- 输入即模糊过滤(名称+镜像/namespace),`↑↓` 选择
- `Enter` 抓取该目标最近 100000 行日志,无缝切入检索界面
- `Ctrl+R` 刷新列表;抓取失败(如 daemon 未启动)显示错误并停留,可重试或换目标
- **反悔随时回去**:检索阶段再按 `Ctrl+R`(或 `Alt+R`)返回选择器重新选目标——列表会重新加载,容器可能已增减;命令面板(`:`)里也有「重新选择容器/Pod」

## 跟随模式

默认实时跟随(`docker logs -f --tail` 同款体验):日志持续写入,界面每 800ms 检测增长并自动重跑当前查询,**保持你的选中位置**,状态栏显示「* 跟随 / 大小」。`--snapshot` 退回一次性快照。

## 搜索日志

- **留空即全量**:不输入关键词直接展示抓到的全部日志,输入后实时过滤,清空回到全量
- **最新优先**:命中数很多时保留**最新的 5000 条**(旧的滚出),配合 [Ctrl+T 筛选](/guides/filtering/)进一步收窄
- 快照自带时间戳(docker/kubectl `--timestamps`),时间筛选可用
- `Enter` 把选中的日志行文本输出到 stdout;快照文件退出自动清理
- 超 1MB 的大日志照样预览:只渲染命中行前后 40/80 行上下文(真实行号 + `...` 标记)

预览正文中日志级别自动着色:ERROR/FATAL/PANIC 红、WARN 黄、INFO/DEBUG 暗。
