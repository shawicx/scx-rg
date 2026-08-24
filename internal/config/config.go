// Package config 读取用户配置文件（~/.config/scx-rg/config.toml）。
// 未配置与配置损坏都回退默认值——配置是增强项，不是启动前提。
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Theme 主题（preset 为命名主题；其余为 hex 覆盖色，优先于 preset）。
type Theme struct {
	Preset    string `toml:"preset"`     // default | dracula | nord | catppuccin（空 = default）
	Accent    string `toml:"accent"`     // 标题底色 / 激活边框 / 选中行
	Match     string `toml:"match"`      // 命中高亮 / 输入提示符
	RowMarker string `toml:"row_marker"` // 行标记 > ✓
}

// History 搜索历史（Ctrl+G 快速调用；落盘 XDG state 目录，退出时写入）。
type History struct {
	Size int `toml:"size"` // 保留条数，0 = 默认 100
}

// Git Git 集成行为。
type Git struct {
	// ShowBlame 状态栏显示选中行 blame 摘要（Ctrl+B 可即时开关）；
	// nil / 缺省 = true（bool 零值无法区分「未配置」与「显式 false」，用指针）
	ShowBlame *bool `toml:"show_blame"`
}

// Editor 外部编辑器（Ctrl+E 打开选中文件到对应行）。
// Command 为空 = 未配置，键位隐藏；Args 支持 {file} {line} 模板变量，
// 留空时按 Command 名称套用 nvim/vim/code/emacs 预置模板。
type Editor struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
}

// Config 用户配置；零值字段在 Load 时回退默认。
type Config struct {
	DebounceMS int      `toml:"debounce_ms"`
	Ignore     []string `toml:"ignore"`
	Theme      Theme    `toml:"theme"`
	Editor     Editor   `toml:"editor"`
	History    History  `toml:"history"`
	Git        Git      `toml:"git"`
}

// Default 内置默认值。
func Default() Config {
	return Config{DebounceMS: 200}
}

// Path 配置文件路径：$XDG_CONFIG_HOME/scx-rg/config.toml 或 ~/.config/scx-rg/config.toml。
func Path() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "scx-rg", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "scx-rg", "config.toml")
}

// Load 读取配置；path 为空时用 Path()。文件不存在返回默认值（常态），
// 解析失败在 stderr 警告后返回默认值，不阻断启动。
func Load(path string) Config {
	cfg := Default()
	if path == "" {
		path = Path()
	}
	if path == "" {
		return cfg
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	var file Config
	if _, err := toml.Decode(string(raw), &file); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ 配置文件 %s 解析失败，使用默认配置: %v\n", path, err)
		return cfg
	}
	if file.DebounceMS > 0 {
		cfg.DebounceMS = file.DebounceMS
	}
	cfg.Ignore = file.Ignore
	cfg.Theme = file.Theme
	cfg.Editor = file.Editor
	cfg.History = file.History
	cfg.Git = file.Git
	return cfg
}
