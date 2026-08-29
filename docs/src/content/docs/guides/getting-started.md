---
title: 开始使用
description: 安装 scx-rg、跑起来第一次搜索。
---

## 安装

一键安装(自动识别平台,下载最新 Release 并校验 sha256,装到 `/usr/local/bin`):

```bash wrap
curl -fsSL https://raw.githubusercontent.com/shawricx/scx-rg/main/scripts/install.sh | sh
# 或装到自定义目录
curl -fsSL https://raw.githubusercontent.com/shawricx/scx-rg/main/scripts/install.sh | sh -s -- --bin ~/.local/bin
```

或手动从 [Releases](https://github.com/shawricx/scx-rg/releases) 下载对应平台的 `scx-rg_<版本>_<os>_<arch>.tar.gz`(macOS / Linux × amd64 / arm64):

```bash
tar -xzf scx-rg_0.0.1_darwin_arm64.tar.gz
sudo mv scx-rg /usr/local/bin/
scx-rg --version    # scx-rg 0.0.1 (commit …, built …, darwin/arm64)
```

建议先核对校验和(与压缩包同目录的 `scx-rg_<版本>_checksums.txt`):

```bash
shasum -a 256 --check scx-rg_0.0.1_checksums.txt --ignore-missing
```

macOS 首次运行未签名二进制若被 Gatekeeper 拦截:

```bash
xattr -d com.apple.quarantine /usr/local/bin/scx-rg
```

## 依赖

- **文件模式**(默认)开箱即用,无需任何依赖。
- **内容模式**(全文搜索)需要 [ripgrep](https://github.com/BurntSushi/ripgrep):`brew install ripgrep`(macOS)或 `apt install ripgrep`(Linux)。

## 第一次搜索

```bash
scx-rg                        # 当前目录,文件模式:输入即搜文件名
scx-rg -path ~/code           # 指定目录
scx-rg -mode content          # 内容模式:全文搜索
```

- 直接输入字符实时搜索(200ms 防抖),`↑↓` 选择,右侧预览自动跟随。
- `Tab` 在文件/内容模式间切换,`Ctrl+F` 切换精确/模糊(文件)或字面量/正则(内容)。
- `?`(输入为空时)或 `F1` 随时打开键位帮助。
- `Enter` 退出并把选中文件的绝对路径打印到 stdout,`Esc` 清空输入、再按退出。

## 接入管道

输出契约是纯文本,天然配合命令替换:

```bash
vim "$(scx-rg -mode content -q TODO)"   # 搜索并直接编辑
```

## 下一步

- [日志](/guides/logs/):Docker / K8s 实时多面板、落盘文件检索、本地日志跟随
- [结果筛选](/guides/filtering/):Ctrl+T 按时间窗、条数可视化过滤
- [键位参考](/guides/keybindings/):完整键位表与堡垒机逃生门
