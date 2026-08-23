package preview

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/alecthomas/chroma/v2"
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
	// 防止极端超长单行（如压缩 JSON 日志）撑爆 viewport；超出以 ... 标记
	maxWrapSegments = 10
)

var (
	styleLineNum   = lipgloss.NewStyle().Foreground(lipgloss.Color("#5B5B72"))
	styleJumpNum   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#3DDC97"))
	styleGutterSep = lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3A4E"))
	styleNotice    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C"))
	styleEllipsis  = lipgloss.NewStyle().Foreground(lipgloss.Color("#5B5B72"))
	// styleHit 预览正文内命中词样式：与列表命中同色系，加下划线以叠加在语法色上
	styleHit = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("#56C9F4"))
)

// codeLine 预览中的一行；no 为真实行号，0 表示跳过分隔行。
type codeLine struct {
	no   int
	text string
}

func renderCode(path string, width, jump int, query string) (Rendered, error) {
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
		return renderCodeWindow(f, lang, st.Size(), width, jump, query)
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
	content, jumpOffset := renderLines(lines, lang, width, jump, query)
	return Rendered{Kind: KindCode, Content: content, JumpLine: jump, JumpOffset: jumpOffset, Lang: lang}, nil
}

// renderCodeWindow 流式扫描大文件，只渲染 jump 行附近的窗口（或无 jump 时的文件头）。
func renderCodeWindow(f *os.File, lang string, size int64, width, jump int, query string) (Rendered, error) {
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
		content, _ := renderLines(lines, lang, width, 0, query)
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
		lines = append(lines, codeLine{no: 0, text: fmt.Sprintf("... 前面省略 %d 行 ...", startNo-1)})
	}
	for i, t := range before {
		lines = append(lines, codeLine{no: startNo + i, text: t})
	}
	lines = append(lines, win...)
	if tail {
		lines = append(lines, codeLine{no: 0, text: "... 后续省略 ..."})
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
		content, _ := renderLines(lines, lang, width, 0, query)
		return Rendered{Kind: KindCode, Content: truncate.String(notice, uint(width)) + "\n" + content, Lang: lang}, nil
	}
	content, jumpOffset := renderLines(lines, lang, width, jump, query)
	// notice 自占一行，jump 物理行号 +1
	return Rendered{
		Kind:       KindCode,
		Content:    truncate.String(notice, uint(width)) + "\n" + content,
		JumpLine:   jump,
		JumpOffset: jumpOffset + 1,
		Lang:       lang,
	}, nil
}

// renderLines 渲染行号槽 + 高亮正文。query 非空时把正文中的命中词（忽略大小写）
// 以 styleHit 标出。长行按面板宽度 ANSI-aware 硬折行，行号只在每段首行显示、
// 续行以空格对齐。返回内容与 jump 行的物理行号（1 起始；jump<=0 时为 0）。
func renderLines(lines []codeLine, lang string, width, jump int, query string) (string, int) {
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
		if query != "" && l.no != 0 { // 分隔行（⋯）不做命中高亮
			body = highlightTermANSI(body, query)
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
			b.WriteString(styleGutterSep.Render(" | ")) // ASCII 分隔符：│ 是歧义宽字符
			b.WriteString(seg)
			b.WriteString("\n")
		}
	}
	return strings.TrimSuffix(b.String(), "\n"), jumpOffset
}

// wrapSegments 把单行按 limit 硬折行（保留 ANSI 样式并在换行处重开），
// 最多 maxWrapSegments 段，超出部分以 ... 收尾。
func wrapSegments(s string, limit int) []string {
	if limit <= 0 {
		return []string{s}
	}
	wrapped := wrap.String(s, limit)
	segs := strings.Split(wrapped, "\n")
	if len(segs) > maxWrapSegments {
		segs = segs[:maxWrapSegments]
		// 末段先腾出省略标记的宽度再追加：截断标记本身不能把行顶超面板宽，
		// 否则超宽行会触发终端软换行、渲染器错位
		last := segs[maxWrapSegments-1]
		if room := limit - lipgloss.Width(" ..."); room > 0 {
			last = truncate.String(last, uint(room))
		}
		segs[maxWrapSegments-1] = last + styleEllipsis.Render(" ...")
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

// formatterFor 决定色彩档位；测试里可覆盖，在无 TTY 环境强制出色彩。
var formatterFor = formatterName

// highlightLines 返回逐行自包含的高亮结果：按 token 边界拆行、每行独立 Format，
// 行内首 token 自带 SGR。整文 format 再按 \n 切分的旧做法会让跨行 token（块注释、
// 多行字符串）的颜色泄漏到相邻行或经 gutter 的 reset 后丢色。返回切片长度与
// strings.Split(text, "\n") 严格一致。
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
	f := formatters.Get(formatterFor())
	if f == nil {
		return fallback
	}
	style := styles.Get("monokai")

	// token 值内的换行切成行边界，类型保留：多行 token 拆成多行内的普通 token
	var lines [][]chroma.Token
	for tok := it(); tok != chroma.EOF; tok = it() {
		if tok.Value == "" {
			continue
		}
		if len(lines) == 0 {
			lines = append(lines, nil)
		}
		parts := strings.Split(tok.Value, "\n")
		for i, p := range parts {
			if i > 0 {
				lines = append(lines, nil)
			}
			if p != "" {
				lines[len(lines)-1] = append(lines[len(lines)-1], chroma.Token{Type: tok.Type, Value: p})
			}
		}
	}
	if len(lines) == 0 {
		return fallback
	}
	out := make([]string, len(lines))
	for i, toks := range lines {
		if len(toks) == 0 {
			out[i] = ""
			continue
		}
		var buf bytes.Buffer
		if err := f.Format(&buf, style, chroma.Literator(toks...)); err != nil {
			return fallback
		}
		out[i] = strings.TrimSuffix(buf.String(), "\n")
	}
	return out
}

// highlightTermANSI 在已含 ANSI 样式的行内把 query 的出现（忽略大小写）以
// styleHit 标出。只在不含转义序列的文本段内匹配（不跨样式边界）；命中后重开
// 之前生效的 SGR，保持后续语法着色不被命中样式结尾的 reset 截断。
func highlightTermANSI(line, query string) string {
	q := []rune(query)
	if len(q) == 0 {
		return line
	}
	var b strings.Builder
	lastSGR := ""
	for i := 0; i < len(line); {
		if line[i] == 0x1b {
			end := escEnd(line, i)
			esc := line[i:end]
			b.WriteString(esc)
			if isSGR(esc) {
				lastSGR = esc
			}
			i = end
			continue
		}
		j := i
		for j < len(line) && line[j] != 0x1b {
			j++
		}
		b.WriteString(highlightRunes(line[i:j], q, lastSGR))
		i = j
	}
	return b.String()
}

// highlightRunes 在不含转义序列的文本段内做忽略大小写匹配并包裹命中片段。
func highlightRunes(text string, q []rune, reopen string) string {
	rs := []rune(text)
	var b strings.Builder
	base := 0
	for k := 0; k+len(q) <= len(rs); k++ {
		if !matchFold(rs[k:k+len(q)], q) {
			continue
		}
		b.WriteString(string(rs[base:k]))
		b.WriteString(styleHit.Render(string(rs[k : k+len(q)])))
		if reopen != "" {
			b.WriteString(reopen)
		}
		k += len(q)
		base = k
	}
	b.WriteString(string(rs[base:]))
	return b.String()
}

// matchFold 逐 rune 简单大小写折叠比较（与列表高亮的 ToLower 语义一致，无分配）。
func matchFold(s, q []rune) bool {
	for i, r := range q {
		if !runeFoldEq(s[i], r) {
			return false
		}
	}
	return true
}

func runeFoldEq(a, b rune) bool {
	if a == b {
		return true
	}
	for f := unicode.SimpleFold(a); f != a; f = unicode.SimpleFold(f) {
		if f == b {
			return true
		}
	}
	return false
}

// escEnd 返回 line[i:] 处转义序列的结束位置（开区间）：
// CSI（ESC [ … 0x40–0x7E 终字节）按终字节收束，其余按两字节转义处理。
func escEnd(line string, i int) int {
	if i+1 < len(line) && line[i+1] == '[' {
		for j := i + 2; j < len(line); j++ {
			if line[j] >= 0x40 && line[j] <= 0x7e {
				return j + 1
			}
		}
		return len(line) // 未终结的序列，吞到行尾
	}
	if i+1 < len(line) {
		return i + 2
	}
	return i + 1
}

func isSGR(esc string) bool {
	return strings.HasPrefix(esc, "\x1b[") && strings.HasSuffix(esc, "m")
}
