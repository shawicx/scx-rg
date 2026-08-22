package preview

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// bigLogFile 生成一个超过 maxCodeBytes 的多行文件，第 n 行内容含 MARK%04d 标记。
func bigLogFile(t *testing.T, lines int) string {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= lines; i++ {
		b.WriteString(fmt.Sprintf("MARK%04d ", i))
		b.WriteString(strings.Repeat("x", 600))
		b.WriteString("\n")
	}
	p := filepath.Join(t.TempDir(), "big.log")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRenderCodeWrapsLongLines(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&b, "line %02d %s\n", i, strings.Repeat("y", 300))
	}
	p := filepath.Join(t.TempDir(), "long.log")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	ren, err := Render(p, 80, 10, ProtocolNone, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	phys := strings.Split(ren.Content, "\n")
	if len(phys) <= 10 {
		t.Fatalf("300 字符长行应折成多段（10 源行 → 物理行 %d）", len(phys))
	}
	for i, l := range phys {
		if w := lipgloss.Width(l); w > 80 {
			t.Errorf("物理行 %d 宽度 %d 超过面板宽度 80", i+1, w)
		}
	}
}

func TestRenderCodeLargeFileWindowAroundJump(t *testing.T) {
	p := bigLogFile(t, 3000) // ~1.8MB
	ren, err := Render(p, 100, 40, ProtocolNone, 2500, "")
	if err != nil {
		t.Fatal(err)
	}
	if ren.Kind != KindCode {
		t.Fatalf("大文件应窗口渲染，Kind = %s", ren.Kind)
	}
	if ren.JumpLine != 2500 {
		t.Fatalf("JumpLine = %d, 期望 2500", ren.JumpLine)
	}
	// jump 行在渲染内容中的物理行（折行后不再等于固定值），验证指向正确：
	// 第 JumpOffset 行应带 jump 的行号 gutter
	if ren.JumpOffset <= 1 {
		t.Fatalf("JumpOffset 应为正数: %d", ren.JumpOffset)
	}
	phys := strings.Split(ren.Content, "\n")
	if len(phys) < ren.JumpOffset {
		t.Fatalf("JumpOffset %d 超出物理行数 %d", ren.JumpOffset, len(phys))
	}
	if !strings.Contains(phys[ren.JumpOffset-1], "2500") {
		t.Fatalf("第 JumpOffset(%d) 行应包含 jump 行号 2500:\n%s", ren.JumpOffset, phys[ren.JumpOffset-1])
	}
	// 行号与内容必须严格对齐（防 jump 行重复进入 before 导致整体错位）：
	// 含 MARK2499 的物理行（源 2499 行的首段）应同时带行号 gutter 2499
	aligned := false
	for _, l := range phys {
		if strings.Contains(l, "MARK2499") {
			aligned = strings.Contains(l, "2499")
			break
		}
	}
	if !aligned {
		t.Fatal("源 2499 行的渲染段应与其行号 gutter 对齐（2499/MARK2499 同行）")
	}
	for _, want := range []string{"MARK2500", "2460", "2500"} { // 命中行、窗口起点行号、命中行号
		if !strings.Contains(ren.Content, want) {
			t.Errorf("窗口内容应包含 %q", want)
		}
	}
	if strings.Contains(ren.Content, "MARK0001") {
		t.Error("窗口外的行（第 1 行）不应出现")
	}
	if !strings.Contains(ren.Content, "前面省略") {
		t.Error("跳过的区间应有省略分隔标记")
	}
}

func TestRenderCodeLargeFileWithoutJumpShowsHead(t *testing.T) {
	p := bigLogFile(t, 3000)
	ren, err := Render(p, 100, 40, ProtocolNone, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if ren.Kind != KindCode {
		t.Fatalf("Kind = %s, 期望 code", ren.Kind)
	}
	if !strings.Contains(ren.Content, "MARK0001") {
		t.Error("无 jump 时应从文件头开始渲染")
	}
	if !strings.Contains(ren.Content, "仅显示") {
		t.Error("应有截断提示")
	}
}

func TestRenderCodeSmallFileRendersFully(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.go")
	if err := os.WriteFile(p, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ren, err := Render(p, 80, 10, ProtocolNone, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if ren.Kind != KindCode || !strings.Contains(ren.Content, "func main()") {
		t.Fatalf("小文件应全量渲染: %+v", ren)
	}
	if ren.Lang != "Go" {
		t.Fatalf("Lang = %q, 期望 Go", ren.Lang)
	}
}

func TestRenderCodeBinaryDetected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bin.dat")
	if err := os.WriteFile(p, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	ren, err := Render(p, 80, 10, ProtocolNone, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if ren.Kind != KindBinary {
		t.Fatalf("Kind = %s, 期望 binary", ren.Kind)
	}
}

// forceColorFormatter 在测试里强制 256 色档位（CI 无 TTY 时 profile 为 Ascii，不出色彩）。
func forceColorFormatter(t *testing.T) {
	t.Helper()
	old := formatterFor
	formatterFor = func() string { return "terminal256" }
	t.Cleanup(func() { formatterFor = old })
}

// forceColor 同时强制 chroma 与 lipgloss 出色彩；断言 ANSI 序列的测试必须用它，
// 否则 Ascii 档位下样式静默退化成纯文本，断言形同虚设。
func forceColor(t *testing.T) {
	t.Helper()
	forceColorFormatter(t)
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}

// TestHighlightLinesSelfContainedPerLine 跨行 token（块注释、多行字符串）修复后，
// 每个受影响行都必须自带 SGR——不再依赖上一行的颜色状态。
func TestHighlightLinesSelfContainedPerLine(t *testing.T) {
	forceColorFormatter(t)
	src := "package main\n\n/*\n块注释甲\n块注释乙\n*/\nvar s = `raw\n跨行字符串`\nfunc main() {}\n"
	hl := highlightLines(src, "Go")
	plain := strings.Split(src, "\n")
	if len(hl) != len(plain) {
		t.Fatalf("高亮行数 %d 应与源行数 %d 严格一致", len(hl), len(plain))
	}
	// 注释续行与字符串续行是旧实现丢色的重灾区（SGR 只在首行出现）
	for _, want := range []string{"块注释甲", "块注释乙", "*/", "跨行字符串`"} {
		i := -1
		for j, l := range plain {
			if strings.Contains(l, want) {
				i = j
				break
			}
		}
		if i < 0 {
			t.Fatalf("源中未找到 %q", want)
		}
		if !strings.Contains(hl[i], "\x1b[") {
			t.Errorf("%q（第 %d 行）未携带自己的 SGR，颜色依赖了上一行: %q", want, i+1, hl[i])
		}
	}
}

func TestRenderHighlightsQueryTerm(t *testing.T) {
	forceColor(t)
	p := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(p, []byte("the Quick brown FOX\nplain line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ren, err := Render(p, 80, 10, ProtocolNone, 1, "quick")
	if err != nil {
		t.Fatal(err)
	}
	if want := styleHit.Render("Quick"); !strings.Contains(ren.Content, want) {
		t.Errorf("预览正文应忽略大小写高亮命中词:\n%s", ren.Content)
	}
	if strings.Contains(ren.Content, styleHit.Render("plain")) {
		t.Error("未命中行不应被高亮")
	}
	// 空 query（文件模式）不应产生命中样式
	ren2, err := Render(p, 80, 10, ProtocolNone, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ren2.Content, styleHit.Render("Quick")) {
		t.Error("空 query 不应高亮命中词")
	}
}

// TestHighlightTermANSIReopensStyle 命中高亮不能截断行内已有的语法着色：
// 命中片段之后需重开之前的 SGR。
func TestHighlightTermANSIReopensStyle(t *testing.T) {
	forceColor(t)
	line := "\x1b[38;5;75mhello QUICK world\x1b[0m"
	got := highlightTermANSI(line, "quick")
	want := "\x1b[38;5;75mhello " + styleHit.Render("QUICK") + "\x1b[38;5;75m world\x1b[0m"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestHighlightTermANSINoMatch(t *testing.T) {
	if got := highlightTermANSI("abc def", "xyz"); got != "abc def" {
		t.Fatalf("无命中应原样返回: %q", got)
	}
	if got := highlightTermANSI("abc", ""); got != "abc" {
		t.Fatalf("空 query 应原样返回: %q", got)
	}
}

// TestRenderCodeCJKAndEmojiWrapWidth 全角/emoji 的折行宽度：
// reflow（runewidth 计宽）与 lipgloss（uniseg 计宽）口径不一致时会在此暴露。
func TestRenderCodeCJKAndEmojiWrapWidth(t *testing.T) {
	forceColorFormatter(t)
	var b strings.Builder
	b.WriteString(strings.Repeat("中文宽度验证", 40) + "\n")              // 全角 240 宽
	b.WriteString("mixed 中英混排 " + strings.Repeat("ＡＢＣ", 80) + "\n") // 全角字母
	b.WriteString(strings.Repeat("🚀🎉✅", 60) + "\n")                 // emoji
	p := filepath.Join(t.TempDir(), "cjk.txt")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	ren, err := Render(p, 80, 10, ProtocolNone, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range strings.Split(ren.Content, "\n") {
		if w := lipgloss.Width(l); w > 80 {
			t.Errorf("物理行 %d 宽度 %d 超过 80: %q", i+1, w, l)
		}
	}
}

func TestRenderCodeChinesePath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "中文目录-文件.go")
	if err := os.WriteFile(p, []byte("package main\n\nfunc 主函数() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ren, err := Render(p, 80, 10, ProtocolNone, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if ren.Kind != KindCode || ren.Lang != "Go" {
		t.Fatalf("中文路径应正常识别与渲染: %+v", ren)
	}
	if !strings.Contains(ren.Content, "主函数") {
		t.Fatalf("内容应包含中文标识符:\n%s", ren.Content)
	}
}
