// Package config 读取用户配置文件（~/.config/scx-rg/config.toml）。
// 未配置与配置损坏都回退默认值——配置是增强项，不是启动前提。
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Theme 主题色（hex 字符串）。
type Theme struct {
	Accent    string `toml:"accent"`     // 标题底色 / 激活边框 / 选中行
	Match     string `toml:"match"`      // 命中高亮 / 输入提示符
	RowMarker string `toml:"row_marker"` // 行标记 > ✓
}

// Config 用户配置；零值字段在 Load 时回退默认。
type Config struct {
	DebounceMS int      `toml:"debounce_ms"`
	Ignore     []string `toml:"ignore"`
	Theme      Theme    `toml:"theme"`
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
	return cfg
}
