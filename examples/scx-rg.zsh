# scx-rg shell 集成示例（zsh）：source 本文件即可启用
#   CTRL-T：fd 文件列表 → 选中插入命令行（支持 Ctrl+Space 多选，多文件依次插入）
#   CTRL-R：命令历史模糊搜索 → 选中替换命令行
# 依赖：fd（brew install fd；也可换成 git ls-files 等任意列路径的命令）

__scx_rg_pick() {
	local out
	out=$("$@" 2>/dev/null | scx-rg --provider stdin 2>/dev/null)
	[[ -n "$out" ]] || return
	LBUFFER+=$(printf '%q ' "$out")
	zle reset-prompt
}

__scx_rg_file() { __scx_rg_pick fd --type f }

__scx_rg_history() {
	local out
	out=$(fc -l 1 | sed 's/^[[:space:]]*[0-9]*[[:space:]]*//' | scx-rg --provider stdin 2>/dev/null)
	[[ -n "$out" ]] || return
	BUFFER=$out
	zle reset-prompt
}

zle -N __scx_rg_file
zle -N __scx_rg_history
bindkey '^T' __scx_rg_file
bindkey '^R' __scx_rg_history
