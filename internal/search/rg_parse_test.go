package search

import (
	"strings"
	"testing"
)

// parseRgLine 纯单测：不依赖真实 rg，CI 恒跑（进程级测试在无 rg 环境会 skip）。
func TestParseRgLine(t *testing.T) {
	cases := []struct {
		name string
		line string
		want Result
		ok   bool
	}{
		{
			name: "match 单行",
			line: `{"type":"match","data":{"path":{"text":"./app/main.go"},"lines":{"text":"func main() {\n"},"line_number":3,"absolute_offset":40,"submatches":[]}}`,
			want: Result{Path: "app/main.go", Line: 3, Text: "func main() {"},
			ok:   true,
		},
		{
			name: "match 多行文本保留内部换行",
			line: `{"type":"match","data":{"path":{"text":"log.txt"},"lines":{"text":"first\nsecond\n"},"line_number":7}}`,
			want: Result{Path: "log.txt", Line: 7, Text: "first\nsecond"},
			ok:   true,
		},
		{
			name: "begin 事件忽略",
			line: `{"type":"begin","data":{"path":{"text":"app/main.go"}}}`,
		},
		{
			name: "context 事件忽略",
			line: `{"type":"context","data":{"path":{"text":"app/main.go"},"lines":{"text":"ctx\n"},"line_number":2}}`,
		},
		{
			name: "end 事件忽略",
			line: `{"type":"end","data":{"path":{"text":"app/main.go"},"binary_offset":null,"stats":{}}}`,
		},
		{
			name: "summary 事件忽略",
			line: `{"type":"summary","data":{"elapsed_total":{"secs":0,"nanos":1,"human":"0s"},"stats":{}}}`,
		},
		{
			name: "空 path 丢弃",
			line: `{"type":"match","data":{"path":{"text":"./"},"lines":{"text":"x\n"},"line_number":1}}`,
		},
		{name: "坏 JSON 跳过", line: `{"type":"match","data":`},
	}
	for _, c := range cases {
		got, ok := parseRgLine([]byte(c.line))
		if ok != c.ok {
			t.Errorf("%s: ok = %v, 期望 %v", c.name, ok, c.ok)
			continue
		}
		if !c.ok {
			continue
		}
		if got.Path != c.want.Path || got.Line != c.want.Line || got.Text != c.want.Text {
			t.Errorf("%s: got {Path:%q Line:%d Text:%q}, 期望 %+v", c.name, got.Path, got.Line, got.Text, c.want)
		}
	}
}

// Text 的 TrimSpace 语义：rg 的 lines.text 总以换行结尾，首尾空白应去掉、
// 中间内容原样保留（多行 match 的后续行同样参与展示）。
func TestParseRgLineTrimsOuterWhitespaceOnly(t *testing.T) {
	line := `{"type":"match","data":{"path":{"text":"a.log"},"lines":{"text":"  indented tail\n"},"line_number":9}}`
	got, ok := parseRgLine([]byte(line))
	if !ok {
		t.Fatal("应为合法 match 事件")
	}
	if got.Text != "indented tail" {
		t.Errorf("Text = %q, 期望首尾空白被去除", got.Text)
	}
	if strings.HasPrefix(got.Text, " ") || strings.HasSuffix(got.Text, "\n") {
		t.Errorf("Text 不应保留首尾空白: %q", got.Text)
	}
}
