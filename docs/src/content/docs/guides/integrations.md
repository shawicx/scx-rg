---
title: 集成与组合
description: 管道、通用 finder、编辑器、shell 集成、Git 增强。
---

## 剪贴板与翻页器

全屏 TUI 里终端原生选择会框住整个界面,两个出口:

- **`Ctrl+O` 翻页器**:在 less 中打开当前预览文件(自动定位选中行)——随意滚动、选择、复制,还能用 less 的 `/` 搜索,`q` 返回
- **`Ctrl+Y` 剪贴板**:走 OSC 52 转义序列,SSH 远程会话同样有效

:::note[OSC 52 前提]
iTerm2 需开启 `Applications in terminal may access clipboard`;tmux 需 `set -g set-clipboard on`。
:::

## 编辑器集成

配置 `[editor]` 后 `Ctrl+E` 把选中文件在编辑器中打开到对应行(TUI 暂停,编辑器退出自动返回):

```toml
[editor]
command = "nvim"
args = ["+{line}", "{file}"]   # {file}=绝对路径 {line}=行号;留空按命令名套预置
```

nvim/vim/emacs/code/zed 有预置参数,`command` 留空则隐藏该键位。检测到 `$NVIM`(nvim `--listen` 会话)时改为发送 quickfix,不打断编辑。

## 管道输出

输入为空按 `|`,输入任意 shell 命令:结果行经 stdin 喂给它,`{path}` `{line}` `{text}` 占位符按当前选中项替换(标记项优先),stdout+stderr 写回预览面板——不离开 TUI。

```text
| jq . > /tmp/out.json        # 例:当前 JSON 行格式化后落盘
```

## 通用 finder(--provider)

把任意「一行一候选」的输出交给 scx-rg 做模糊筛选,`Enter` 输出选中行文本(支持多选):

```bash
fd --type f | scx-rg --provider stdin        # 文件选择(候选是真实路径时自动获得预览)
git branch | scx-rg --provider stdin         # 任意列表
scx-rg --provider docker-ps                  # 内置:docker 容器(镜像·状态做详情)
docker stop $(scx-rg --provider docker-ps)   # 组合用法
```

## shell 键绑定

[examples/scx-rg.zsh](https://github.com/shawricx/scx-rg/blob/main/examples/scx-rg.zsh)(及 fish 版)提供 fzf 式键绑定:

```zsh
# ~/.zshrc
source /path/to/scx-rg/examples/scx-rg.zsh
```

- `CTRL-T`:从 `fd` 文件列表选文件插入命令行
- `CTRL-R`:模糊搜索命令历史,选中替换命令行

## Git 增强

- **Blame 摘要**:git 仓库内状态栏显示当前选中行的 `短hash 作者 时间`;`Ctrl+B` 开关,整文件按修改时间缓存
- **Git 历史搜索**:命令面板(`:`)→「Git 历史」,`git log -G<关键词>` 流式列出引入/删除该代码的提交,右侧显示 commit 详情,`Enter` 复制短 hash
- **AST 批量替换**:输入为空按 `R` 进入 ast-grep 替换——AST 模式(`$VAR` 元变量)→ 重写模板 → 匹配列表 + `-旧/+新` diff 预览,`y` 应用当前 / `a` 全部 / `n` 跳过。要求 git **干净工作区**(审查与回滚交给 `git diff` / `git checkout -- .`);需要 [ast-grep](https://ast-grep.com)(`brew install ast-grep`),未安装时该命令自动隐藏
