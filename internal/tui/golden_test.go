package tui

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"scx-rg/internal/preview"
	"scx-rg/internal/search"
)

// goldenUpdate 刷新基线：go test ./internal/tui -update
var goldenUpdate = flag.Bool("update", false, "更新 golden frame 基线文件")

// ansiRe 剥离帧中的 ANSI 序列：CSI（SGR/光标）、OSC（剪贴板等）、
// kitty 图形 APC。golden 对比的是纯文本布局。
var ansiRe = regexp.MustCompile("\x1b\\[[0-9;:?]*[a-zA-Z]|\x1b\\][^\x07\x1b]*(\x07|\x1b\\\\)|\x1b_G[^\x1b]*\x1b\\\\")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// goldenFrame 渲染帧与基线对比（或 -update 时写入基线）。
// 场景必须确定性：rg-free（walkFiles 排序稳定）、无时间戳、无随机路径
// （列表/预览显示的是相对路径）。
func goldenFrame(t *testing.T, name, frame string) {
	t.Helper()
	frame = strings.TrimRight(stripANSI(frame), "\n") + "\n"
	path := filepath.Join("testdata", "golden", name+".txt")
	if *goldenUpdate {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(frame), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden 基线缺失（go test ./internal/tui -update 生成）: %v", err)
	}
	if frame != string(want) {
		t.Errorf("帧 %s 与基线不符（确认是预期变化后用 -update 刷新）：\n%s",
			name, goldenDiff(string(want), frame))
	}
}

// goldenDiff 报告首差异行 ±3 行上下文，避免整帧刷屏。
func goldenDiff(want, got string) string {
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	i := 0
	for i < len(wl) && i < len(gl) && wl[i] == gl[i] {
		i++
	}
	lo, hi := max(0, i-3), min(max(len(wl), len(gl)), i+4)
	var b strings.Builder
	fmt.Fprintf(&b, "  第 %d 行起不一致：\n", i+1)
	for j := lo; j < min(hi, len(wl)); j++ {
		fmt.Fprintf(&b, "%s基线 %3d| %s\n", diffMark(j, i), j+1, wl[j])
	}
	for j := lo; j < min(hi, len(gl)); j++ {
		fmt.Fprintf(&b, "%s实际 %3d| %s\n", diffMark(j, i), j+1, gl[j])
	}
	if len(wl) != len(gl) {
		fmt.Fprintf(&b, "  行数：基线 %d / 实际 %d\n", len(wl), len(gl))
	}
	return b.String()
}

func diffMark(j, at int) string {
	if j == at {
		return "-/+"
	}
	return "   "
}

// goldenFilesModel 固定文件集的 files 模型（rg-free：walkFiles 排序稳定）。
func goldenFilesModel(t *testing.T) *Model {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"app.go":      "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n",
		"readme.md":   "# 标题\n\n正文说明，验证中文渲染。\n",
		"sub/util.go": "package sub\n\nfunc Util() {}\n",
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return New(Config{Root: dir, RgAvailable: false})
}

func TestGoldenFilesFilter(t *testing.T) {
	m := goldenFilesModel(t)
	frame := m.RenderOnce(90, 24, "app", "app.go")
	goldenFrame(t, "files_filter", frame)
}

func TestGoldenFinder(t *testing.T) {
	m := New(Config{
		Candidates: []search.Candidate{
			{Text: "web-server", Detail: "nginx:latest · Up 3 days"},
			{Text: "web-worker", Detail: "worker:2.1 · Up 3 days"},
			{Text: "db-primary"},
		},
		FinderName: "docker",
		PickLine:   true,
	})
	_ = m.RenderOnce(90, 24, "web", "") // 过滤出 web-* 两条
	// finder 候选非文件路径，直接跟随首条展示详情面板
	m.prevPath = ""
	m.drain(m.followSelection())
	goldenFrame(t, "finder", m.View())
}

func TestGoldenHelp(t *testing.T) {
	m := goldenFilesModel(t)
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	m.drain(m.Init())
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF1}); cmd != nil {
		m.drain(cmd)
	}
	goldenFrame(t, "help", m.View())
}

func TestGoldenMultiselect(t *testing.T) {
	m := goldenFilesModel(t)
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	m.drain(m.Init())
	for range 2 {
		if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt}); cmd != nil {
			m.drain(cmd)
		}
	}
	goldenFrame(t, "multiselect", m.View())
}

func TestGoldenImagePlaceholder(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "img.png"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{Root: dir, RgAvailable: false, ImgProto: preview.ProtocolNone})
	frame := m.RenderOnce(90, 24, "", "img.png")
	goldenFrame(t, "image_placeholder", frame)
}
