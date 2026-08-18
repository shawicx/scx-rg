package preview

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
	"github.com/muesli/termenv"
)

const (
	maxCodeBytes = 1 << 20 // 超过 1MB 的文本不再高亮
	maxCodeLines = 3000
)

var (
	styleLineNum   = lipgloss.NewStyle().Foreground(lipgloss.Color("#5B5B72"))
	styleJumpNum   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#3DDC97"))
	styleGutterSep = lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3A4E"))
)

func renderCode(path string, width, jump int) (Rendered, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Rendered{Kind: KindMissing, Content: "无法读取文件: " + err.Error()}, nil
	}
	if len(data) > maxCodeBytes {
		return Rendered{Kind: KindTooLarge, Content: fmt.Sprintf("文件过大（%d KB），超过 %d KB 上限，跳过预览", len(data)/1024, maxCodeBytes/1024)}, nil
	}
	if isBinary(data) {
		return Rendered{Kind: KindBinary, Content: "[ 二进制文件 ]"}, nil
	}

	text := strings.ReplaceAll(string(data), "\t", "    ")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(lines) > maxCodeLines {
		lines = lines[:maxCodeLines]
	}

	lang := detectLang(path)
	hl := highlightLines(strings.Join(lines, "\n"), lang)
	if len(hl) > len(lines) { // 高亮输出可能带尾随空行
		hl = hl[:len(lines)]
	}

	numW := len(fmt.Sprintf("%d", len(lines)))
	var b strings.Builder
	for i, hlLine := range hl {
		n := i + 1
		numStr := fmt.Sprintf("%*d", numW, n)
		if n == jump {
			numStr = styleJumpNum.Render(numStr)
		} else {
			numStr = styleLineNum.Render(numStr)
		}
		b.WriteString(numStr)
		b.WriteString(styleGutterSep.Render(" │ "))
		b.WriteString(hlLine)
		b.WriteString("\n")
	}
	content := strings.TrimSuffix(b.String(), "\n")
	if width > 0 {
		content = truncateLines(content, width)
	}
	if jump > len(lines) {
		jump = 0
	}
	return Rendered{Kind: KindCode, Content: content, JumpLine: jump, Lang: lang}, nil
}

func detectLang(path string) string {
	if lx := lexers.Match(path); lx != nil {
		return lx.Config().Name
	}
	return "plaintext"
}

func formatterName() string {
	switch termenv.ColorProfile() {
	case termenv.TrueColor:
		return "terminal16m"
	case termenv.ANSI256:
		return "terminal256"
	case termenv.ANSI:
		return "terminal"
	default:
		return "tty"
	}
}

func highlightLines(text, lang string) []string {
	fallback := strings.Split(text, "\n")
	lx := lexers.Get(lang)
	if lx == nil {
		lx = lexers.Fallback
	}
	it, err := lx.Tokenise(nil, text)
	if err != nil {
		return fallback
	}
	f := formatters.Get(formatterName())
	if f == nil {
		return fallback
	}
	style := styles.Get("monokai")
	var buf bytes.Buffer
	if err := f.Format(&buf, style, it); err != nil {
		return fallback
	}
	return strings.Split(buf.String(), "\n")
}

func truncateLines(content string, width int) string {
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		lines[i] = truncate.String(l, uint(width))
	}
	return strings.Join(lines, "\n")
}

func isBinary(data []byte) bool {
	n := min(len(data), 8192)
	return bytes.IndexByte(data[:n], 0) >= 0
}

var _ = filepath.Ext
