package preview

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// stubProfile 注入色彩档位（测试进程无 TTY，termenv.ColorProfile() 为 Ascii）。
func stubProfile(p termenv.Profile) func() {
	old := colorProfile
	colorProfile = func() termenv.Profile { return p }
	return func() { colorProfile = old }
}

// TestUpperHalfBlockIsSingleWidth 锁死渲染管线的计宽假设：▀（U+2580，East Asian
// Ambiguous）在 lipgloss / x/ansi 口径下按 1 格计宽——tui 帧宽收窄与边框对齐
// 都依赖它。未来 lipgloss 升级若改变口径，此测试先红再改方案。
func TestUpperHalfBlockIsSingleWidth(t *testing.T) {
	if w := lipgloss.Width("▀"); w != 1 {
		t.Fatalf(`lipgloss.Width("▀") = %d, 期望 1（halfblock 行宽假设被破坏）`, w)
	}
}

func solidImage(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestHalfblockTrueColor(t *testing.T) {
	defer stubProfile(termenv.TrueColor)()
	out, err := halfblockBlock(solidImage(2, 2, color.RGBA{255, 0, 0, 255}), 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	// 2×2 像素等比放大到 4×4（cols=4、rows=2 → 每格 2 像素行）→ 2 行 × 4 字符
	if n := strings.Count(out, "▀"); n != 8 {
		t.Errorf("▀ 字符数 = %d, 期望 8:\n%q", n, out)
	}
	if lines := strings.Count(out, "\n") + 1; lines != 2 {
		t.Errorf("输出行数 = %d, 期望 2:\n%q", lines, out)
	}
	if !strings.Contains(out, "38;2;255;0;0") || !strings.Contains(out, "48;2;255;0;0") {
		t.Errorf("truecolor 前景/背景序列缺失:\n%q", out)
	}
	// 同色像素 SGR 压缩：整图纯色，每行只应各设一次前景与背景
	if n := strings.Count(out, "38;2;255;0;0"); n != 2 {
		t.Errorf("纯色图每行只应设一次前景（压缩生效），实际 %d 次", n)
	}
}

func TestHalfblockOddHeightLastRowBlackBg(t *testing.T) {
	defer stubProfile(termenv.TrueColor)()
	out, err := halfblockBlock(solidImage(3, 3, color.RGBA{0, 255, 0, 255}), 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("输出行数 = %d, 期望 2:\n%q", len(lines), out)
	}
	// 奇数像素高：末行下半像素不存在，背景应为黑
	if !strings.Contains(lines[1], "48;2;0;0;0") {
		t.Errorf("末行背景应为黑色:\n%q", lines[1])
	}
}

func TestHalfblockColorDowngrade(t *testing.T) {
	defer stubProfile(termenv.ANSI256)()
	out, err := halfblockBlock(solidImage(2, 2, color.RGBA{255, 0, 0, 255}), 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "38;5;") {
		t.Errorf("ANSI256 档位应输出 38;5;n 序列:\n%q", out)
	}
	if strings.Contains(out, "38;2;") {
		t.Errorf("ANSI256 档位不应残留 truecolor 序列:\n%q", out)
	}
}

func TestHalfblockAsciiFallsBackToPlaceholder(t *testing.T) {
	defer stubProfile(termenv.Ascii)()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "img.png")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	ren, err := Render(path, 40, 20, ProtocolHalfblock, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if ren.Kind != KindImage {
		t.Fatalf("Kind = %q, 期望 image", ren.Kind)
	}
	if !strings.Contains(ren.Content, "色彩能力") {
		t.Errorf("无色输出应回退占位盒说明:\n%s", ren.Content)
	}
}
