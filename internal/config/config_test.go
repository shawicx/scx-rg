package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeCfg(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFileReturnsDefault(t *testing.T) {
	cfg := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if !reflect.DeepEqual(cfg, Default()) {
		t.Errorf("文件不存在应返回默认值: %+v", cfg)
	}
}

func TestLoadParsesFields(t *testing.T) {
	path := writeCfg(t, `
debounce_ms = 350
ignore = ["build", ".venv"]

[theme]
accent = "#FF0000"
match = "#00FF00"
row_marker = "#0000FF"
`)
	cfg := Load(path)
	if cfg.DebounceMS != 350 {
		t.Errorf("debounce_ms = %d, 期望 350", cfg.DebounceMS)
	}
	if len(cfg.Ignore) != 2 || cfg.Ignore[0] != "build" {
		t.Errorf("ignore = %v", cfg.Ignore)
	}
	if cfg.Theme.Accent != "#FF0000" || cfg.Theme.Match != "#00FF00" || cfg.Theme.RowMarker != "#0000FF" {
		t.Errorf("theme = %+v", cfg.Theme)
	}
}

func TestLoadBadTOMLFallsBackToDefault(t *testing.T) {
	cfg := Load(writeCfg(t, "not = [valid toml"))
	if !reflect.DeepEqual(cfg, Default()) {
		t.Errorf("坏文件应回退默认值: %+v", cfg)
	}
}

func TestLoadZeroValuesKeepDefaults(t *testing.T) {
	// 部分配置：debounce_ms 缺省（0）应保持默认 200
	cfg := Load(writeCfg(t, "ignore = [\"x\"]\n"))
	if cfg.DebounceMS != 200 {
		t.Errorf("debounce_ms = %d, 期望默认 200", cfg.DebounceMS)
	}
	if len(cfg.Ignore) != 1 {
		t.Errorf("ignore = %v", cfg.Ignore)
	}
}
