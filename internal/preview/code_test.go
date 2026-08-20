package preview

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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
	ren, err := Render(p, 80, 10, ProtocolNone, 0)
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
	ren, err := Render(p, 100, 40, ProtocolNone, 2500)
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
	if !strings.Contains(ren.Content, "⋯") {
		t.Error("跳过的区间应有 ⋯ 分隔标记")
	}
}

func TestRenderCodeLargeFileWithoutJumpShowsHead(t *testing.T) {
	p := bigLogFile(t, 3000)
	ren, err := Render(p, 100, 40, ProtocolNone, 0)
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
	ren, err := Render(p, 80, 10, ProtocolNone, 3)
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
	ren, err := Render(p, 80, 10, ProtocolNone, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ren.Kind != KindBinary {
		t.Fatalf("Kind = %s, 期望 binary", ren.Kind)
	}
}
