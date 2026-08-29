---
title: 常见问题
description: 剪贴板、键位冲突、图片显示与已知限制。
---

## 复制不工作?

`Ctrl+Y` 走 OSC 52 转义序列:

- iTerm2:开启 *Preferences → General → Selection → Applications in terminal may access clipboard*
- tmux:`set -g set-clipboard on`
- 不满足条件时改用 `Ctrl+O` 翻页器:在 less 里用终端原生选择复制

## 浏览器/堡垒机里 Ctrl 组合键被截获?

三层应对,见[键位参考](/guides/keybindings/#堡垒机-浏览器-web-终端):`Alt+字母` 别名、`:` 命令面板(纯字符键)、环境侧扩展/PWA。

## 图片不显示 / 显示错位?

- SSH 远程:环境变量不透传,本地 kitty 可能被误判为 halfblock——`--img kitty` 强制指定(或在 SSH 配置 `SetEnv KITTY_WINDOW_ID`)
- 开启 iTerm2 *ambiguous double* 选项的终端:`▀` 半块字符按 2 格渲染导致错位,`--img none` 关闭
- 管道/CI 等无色彩输出:自动回退占位盒,属预期行为

## IDE 运行窗里启动异常?

无 TTY 环境(IDE 内置运行窗、部分管道场景)自动降级为单帧渲染,提示后以 `--once` 效果输出。交互模式请在真实终端运行。

## 其他已知限制

- GIF 只显示首帧(动画播放在 backlog)
- 列表无虚拟化,当前上限 500 条(日志模式为最新 5000 条窗口)
- 预览长行按面板宽度折行,单行最多折 10 段、超出以 `...` 标记
- 多目录 workspace 下 Git 筛选段隐藏(文件集口径限制)

## 搜索报错?

- 错误显示在底部状态栏(非法正则、权限问题等);内容模式下非法正则会**自动按字面量兜底**,一般无需手动处理
- rg 退出码 1(无匹配)不会误报为错误
