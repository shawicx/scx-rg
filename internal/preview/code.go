package preview

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
	"github.com/muesli/reflow/wrap"
	"github.com/muesli/termenv"
)

const (
	maxCodeBytes = 1 << 20 // 全量渲染上限；更大的文件走窗口化渲染（日志场景）
	maxCodeLines = 3000
	windowBefore = 40 // jump 行之前的上下文行数
	windowAfter  = 80 // jump 行之后的上下文行数
	// maxWrapSegments 单个源行最多折出的物理段数（约 contentW×10 字符），
	// 防止极端超长单行（如压缩 JSON 日志）撑爆 viewport；超出以 ⋯ 标记
	maxWrapSegments = 10
)

var (
	styleLineNum   = lipgloss.NewStyle().Foreground(lipgloss.Color("#5B5B72"))
	styleJumpNum   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#3DDC97"))
	styleGutterSep = lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3A4E"))
	styleNotice    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C"))
	styleEllipsis  = lipgloss.NewStyle().Foreground(lipgloss.Color("#5B5B72"))
)

// codeLine 预览中的一行；no 为真实行号，0 表示跳过分隔行。
type codeLine struct {
	no   int
	text string
}

func renderCode(path string, width, jump int) (Rendered, error) {
	f, err := os.Open(path)
	if err != nil {
		return Rendered{Kind: KindMissing, Content: "无法读取文件: " + err.Error()}, nil
	}
	defer f.Close()

	sniff := make([]byte, 8192)
	n, _ := f.Read(sniff)
	if bytes.IndexByte(sniff[:n], 0) >= 0 {
		return Rendered{Kind: KindBinary, Content: "[ 二进制文件 ]"}, nil
	}
	st, err := f.Stat()
	if err != nil {
		return Rendered{Kind: KindMissing, Content: "无法读取文件: " + err.Error()}, nil
	}
	lang := detectLang(path)

	if st.Size() > maxCodeBytes {
		return renderCodeWindow(f, lang, st.Size(), width, jump)
	}

	_, _ = f.Seek(0, io.SeekStart)
	data, err := io.ReadAll(f)
	if err != nil {
		return Rendered{Kind: KindMissing, Content: "无法读取文件: " + err.Error()}, nil
	}
	raw := strings.Split(strings.TrimSuffix(expandTabs(string(data)), "\n"), "\n")
	if len(raw) > maxCodeLines {
		raw = raw[:maxCodeLines]
	}
	if jump > len(raw) {
		jump = 0
	}
	lines := make([]codeLine, len(raw))
	for i, t := range raw {
		lines[i] = codeLine{no: i + 1, text: t}
	}
	content, jumpOffset := renderLines(lines, lang, width, jump)
	return Rendered{Kind: KindCode, Content: content, JumpLine: jump, JumpOffset: jumpOffset, Lang: lang}, nil
}

// renderCodeWindow 流式扫描大文件，只渲染 jump 行附近的窗口（或无 jump 时的文件头）。
func renderCodeWindow(f *os.File, lang string, size int64, width, jump int) (Rendered, error) {
	_, _ = f.Seek(0, io.SeekStart)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024) // 日志单行可能很长

	if jump <= 0 {
		notice := styleNotice.Render(fmt.Sprintf("文件较大（%.1f MB）：仅显示前 %d 行，搜索命中后自动定位上下文",
			float64(size)/(1<<20), maxCodeLines))
		var lines []codeLine
		for i := 0; i < maxCodeLines && sc.Scan(); i++ {
			lines = append(lines, codeLine{no: i + 1, text: expandTabs(sc.Text())})
		}
		content, _ := renderLines(lines, lang, width, 0)
		return Rendered{Kind: KindCode, Content: truncate.String(notice, uint(width)) + "\n" + content, Lang: lang}, nil
	}

	notice := styleNotice.Render(fmt.Sprintf("文件较大（%.1f MB）：显示第 %d 行前 %d / 后 %d 行",
		float64(size)/(1<<20), jump, windowBefore, windowAfter))
	var (
		seen   int
		found  bool
		before []string // jump 之前最近 windowBefore 行的滚动缓冲
		win    []codeLine
		tail   bool // 窗口收满但文件还有更多行
	)
	for sc.Scan() {
		seen++
		text := expandTabs(sc.Text())
		if !found {
			if seen == jump {
				found = true
				win = append(win, codeLine{no: seen, text: text}) // jump 行只进 win，不进 before
				continue
			}
			before = append(before, text)
			if len(before) > windowBefore {
				before = before[1:]
			}
			continue
		}
		if len(win) > windowAfter {
			tail = true
			break
		}
		win = append(win, codeLine{no: seen, text: text})
	}

	startNo := jump - len(before)
	lines := make([]codeLine, 0, len(before)+len(win)+2)
	if startNo > 1 {
		lines = append(lines, codeLine{no: 0, text: fmt.Sprintf("⋯ 前面省略 %d 行 ⋯", startNo-1)})
	}
	for i, t := range before {
		lines = append(lines, codeLine{no: startNo + i, text: t})
	}
	lines = append(lines, win...)
	if tail {
		lines = append(lines, codeLine{no: 0, text: "⋯ 后续省略 ⋯"})
	}
	if !found {
		// jump 超出文件末尾：回退显示最后 windowBefore 行
		notice = styleNotice.Render(fmt.Sprintf("文件较大（%.1f MB）：第 %d 行超出文件末尾，显示最后 %d 行",
			float64(size)/(1<<20), jump, len(before)))
		jump = 0
		lines = lines[len(lines)-len(before):]
		if len(lines) == 0 {
			return Rendered{Kind: KindEmpty, Content: truncate.String(notice, uint(width)), Lang: lang}, nil
		}
		content, _ := renderLines(lines, lang, width, 0)
		return Rendered{Kind: KindCode, Content: truncate.String(notice, uint(width)) + "\n" + content, Lang: lang}, nil
	}
	content, jumpOffset := renderLines(lines, lang, width, jump)
	// notice 自占一行，jump 物理行号 +1
	return Rendered{
		Kind:       KindCode,
		Content:    truncate.String(notice, uint(width)) + "\n" + content,
		JumpLine:   jump,
		JumpOffset: jumpOffset + 1,
		Lang:       lang,
	}, nil
}

// renderLines 渲染行号槽 + 高亮正文。长行按面板宽度 ANSI-aware 硬折行，
// 行号只在每段首行显示、续行以空格对齐。返回内容与 jump 行的物理行号
// （1 起始；jump<=0 时为 0）。
func renderLines(lines []codeLine, lang string, width, jump int) (string, int) {
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = l.text
	}
	hl := highlightLines(strings.Join(plain, "\n"), lang)
	numW := 1
	for _, l := range lines {
		if l.no > 0 {
			numW = max(numW, len(fmt.Sprintf("%d", l.no)))
		}
	}
	contentW := 0
	if width > 0 {
		contentW = max(10, width-numW-3)
	}

	var b strings.Builder
	phys := 0
	jumpOffset := 0
	for i, l := range lines {
		body := l.text
		if i < len(hl) && strings.TrimSpace(hl[i]) != "" {
			body = hl[i]
		}
		var segs []string
		switch {
		case l.no == 0: // 跳过分隔行不折
			segs = []string{styleEllipsis.Render(l.text)}
		case contentW > 0:
			segs = wrapSegments(body, contentW)
		default:
			segs = []string{body}
		}
		for si, seg := range segs {
			phys++
			if l.no > 0 && l.no == jump && jumpOffset == 0 {
				jumpOffset = phys
			}
			if l.no == 0 {
				b.WriteString(seg)
				b.WriteString("\n")
				continue
			}
			if si == 0 {
				numStr := fmt.Sprintf("%*d", numW, l.no)
				if l.no == jump {
					numStr = styleJumpNum.Render(numStr)
				} else {
					numStr = styleLineNum.Render(numStr)
				}
				b.WriteString(numStr)
			} else {
				b.WriteString(strings.Repeat(" ", numW))
			}
			b.WriteString(styleGutterSep.Render(" │ "))
			b.WriteString(seg)
			b.WriteString("\n")
		}
	}
	return strings.TrimSuffix(b.String(), "\n"), jumpOffset
}

// wrapSegments 把单行按 limit 硬折行（保留 ANSI 样式并在换行处重开），
// 最多 maxWrapSegments 段，超出部分以 ⋯ 收尾。
func wrapSegments(s string, limit int) []string {
	if limit <= 0 {
		return []string{s}
	}
	wrapped := wrap.String(s, limit)
	segs := strings.Split(wrapped, "\n")
	if len(segs) > maxWrapSegments {
		segs = segs[:maxWrapSegments]
		segs[maxWrapSegments-1] += styleEllipsis.Render(" ⋯")
	}
	return segs
}

func expandTabs(s string) string { return strings.ReplaceAll(s, "\t", "    ") }

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
