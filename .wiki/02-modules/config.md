# config —— 用户配置文件

包路径：`internal/config`。配置是增强项不是启动前提：未配置/损坏一律回退默认，不阻断启动。

## 文件与优先级

路径：`$XDG_CONFIG_HOME/scx-rg/config.toml`，未设 XDG 时 `~/.config/scx-rg/config.toml`（`config.Path()`）。

取值优先级：**flag 显式设置 > config.toml > 内置默认**（main.go 用 `flag.Visit` 区分「显式设置」与「默认值」）。

## 字段

```toml
debounce_ms = 200                  # 搜索防抖（毫秒）；--debounce-ms 显式值优先
ignore = ["build", ".venv"]        # 额外忽略目录名，追加生效（见下）

[theme]
accent     = "#7D56F4"             # 标题底色 / 激活边框 / 选中行
match      = "#56C9F4"             # 命中高亮 / 输入提示符
row_marker = "#3DDC97"             # 行标记 > ✓
```

## 注入路径

| 配置项 | 消费方 | 机制 |
| --- | --- | --- |
| `debounce_ms` | `tui.Config.Debounce` | main 读入，flag 覆盖 |
| `ignore` | `tui.Config.IgnoreDirs` → `FilesProvider.IgnoreExtra` | 双路径生效：`ListFiles` 追加 `rg -g '!dir/'` glob；`walkFiles` 合并判断 |
| `[theme]` 三色 | `tui.ApplyTheme(accent, match, rowMarker)` | 改 styles.go 的 `colorAccent/colorCyan/colorOK` 变量后调 `initStyles()` 重建全部包级样式（唯一定义点，须在 `tui.New` 之前调用） |

不进配置（刻意克制）：MaxResults、预览缓存容量、图片协议（走 `--img` / 自动探测）、preview 包的样式色。

## 行为细节

- 文件不存在 → `Default()`（常态，静默）
- TOML 解析失败 → stderr 警告 + 默认值
- 零值字段回退默认（如 `debounce_ms` 缺省仍 200），只接受显式提供的值
- `Load(path)` 路径参数可注入，测试用临时文件（config_test.go）

Related: [tui（样式与主题）](tui.md) · [README 配置章节](../../README.md)
