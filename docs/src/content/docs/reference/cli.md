---
title: 命令行参数
description: scx-rg 全部 CLI 参数与 docker/k8s 子命令。
---

## 全局

```text
scx-rg [flags]

-path string    搜索根目录(默认 .;配合 --follow 可指向单个日志文件)
-mode string    初始模式:files | content(默认 files)
-img string     图片协议:auto | kitty | sixel | halfblock | none(默认 auto 自动探测)
-provider string  候选来源:stdin | docker-ps(管道候选取代文件搜索,Enter 输出选中行)
-debounce-ms    搜索防抖间隔(默认 200,可被 config.toml 覆盖)
-title string   头部标题(如 docker:web)
--follow        跟随 -path 指定的单个日志文件,实时刷新(tail -f 式)
--version       输出版本信息并退出
```

## docker / k8s 子命令

```bash
scx-rg docker [容器名] [--snapshot]
scx-rg k8s [Pod名] [-n namespace] [-c 容器名] [--snapshot]
```

- 不带名字:进入交互选择器(模糊过滤,免记名字)
- 默认实时跟随(抓最近 100000 行 + 持续追加);`--snapshot` 只抓一次
- 抓取行数上限固定 100000(`--tail` 直传底层命令)

## 调试

```text
--once          渲染一帧后退出(调试/CI 冒烟,不进备用屏)
-q string       配合 --once 模拟搜索词
-preview-file   配合 --once 强制预览指定文件
-w / -h         配合 --once 的渲染尺寸(默认 120x40)
```

## 图片协议自动探测顺序

1. **kitty 图形协议**:环境标志(`KITTY_WINDOW_ID` / `TERM` 含 kitty/ghostty / `WEZTERM_PANE` 等)
2. **sixel**:向终端发 DA1 查询(150ms 超时),响应支持则启用;无响应时回退 `TERM` 启发式(foot / yaft / mlterm)
3. **halfblock**:`▀` 半块字符 + truecolor → 256 色 → 16 色逐级降级,任何彩色终端可用

`--img none` 显式禁用;SSH 远程环境变量不透传时可用 `--img kitty` 强制指定。
