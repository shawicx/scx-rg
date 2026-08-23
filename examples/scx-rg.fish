# scx-rg shell 集成示例（fish）：source 本文件即可启用
#   CTRL-T：fd 文件列表 → 选中插入命令行（支持 Ctrl+Space 多选）
#   CTRL-R：命令历史模糊搜索 → 选中替换命令行
# 依赖：fd（brew install fd）

function scx_rg_file
    set -l out (fd --type f 2>/dev/null | scx-rg --provider stdin 2>/dev/null)
    if test -n "$out"
        commandline -i -- (string escape -- $out)' '
    end
    commandline -f repaint
end

function scx_rg_history
    set -l out (builtin history | scx-rg --provider stdin 2>/dev/null)
    if test -n "$out"
        commandline -r -- $out
    end
    commandline -f repaint
end

bind \ct scx_rg_file
bind \cr scx_rg_history
