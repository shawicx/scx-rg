---
title: 配置文件
description: ~/.config/scx-rg/config.toml 全部选项。
---

路径:`~/.config/scx-rg/config.toml`。未配置时全部使用内置默认;配置损坏回退默认并警告,不阻断启动。

**优先级:命令行 flag 显式设置 > config.toml > 默认。**

```toml
# 搜索防抖间隔(毫秒)
debounce_ms = 200

# 额外忽略的目录名(追加到内置忽略;对 rg 枚举与内置遍历都生效)
ignore = ["build", ".venv"]

[theme]
# 命名主题:default | dracula | nord | catppuccin(空 = default;
# 也可在命令面板 : 循环切换,会话级)
preset = "default"
# 显式 hex 三色,优先于 preset 同槽位
accent     = "#7D56F4"   # 标题底色 / 激活边框 / 选中行
match      = "#56C9F4"   # 命中高亮 / 输入提示符
row_marker = "#3DDC97"   # 行标记 > ✓

[editor]
# Ctrl+E 打开选中文件到对应行;command 留空 = 键位隐藏。
# args 支持 {file}(绝对路径)与 {line} 模板变量;留空时按命令名套用
# nvim/vim/emacs(+行号 文件)、code(--goto 文件:行)、zed(文件:行)预置。
command = "nvim"
args = ["+{line}", "{file}"]

[history]
size = 100   # 搜索历史保留条数(Ctrl+G 调用)

[git]
show_blame = true   # 状态栏 blame 摘要(Ctrl+B 可即时切换)
```

## 主题预览

四套命名主题的区分主要在强调色(选中行、激活边框、命中高亮)。会话内用命令面板(`:`)→「切换主题」循环试,满意后写进 `preset` 固化。
