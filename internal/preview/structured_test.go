package preview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTmp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// JSON：紧凑输入 → 缩进树；JumpLine 恒 0（禁跳转）；Lang 标注 JSON。
func TestStructuredJSONIndent(t *testing.T) {
	p := writeTmp(t, "a.json", `{"b":1,"a":[1,2,{"c":"d"}],"z":"末尾"}`)
	ren, err := Render(p, 80, 24, ProtocolNone, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if ren.Lang != "JSON" {
		t.Errorf("Lang = %q", ren.Lang)
	}
	for _, want := range []string{"\"b\": 1", "\"a\": [", "1,", "\"c\": \"d\""} {
		if !strings.Contains(ren.Content, want) {
			t.Errorf("缩进树应含 %q:\n%s", want, ren.Content)
		}
	}
	if ren.JumpLine != 0 {
		t.Errorf("结构化预览 JumpLine 应为 0: %d", ren.JumpLine)
	}
}

// 非法 JSON 回退普通代码渲染（不报错）。
func TestStructuredJSONFallback(t *testing.T) {
	p := writeTmp(t, "bad.json", `{not json`)
	ren, err := Render(p, 80, 24, ProtocolNone, 0, "")
	if err != nil || ren.Kind != KindCode {
		t.Errorf("非法 JSON 应回退代码渲染: %v %+v", err, ren)
	}
	if !strings.Contains(ren.Content, "{not json") {
		t.Error("回退应显示原文")
	}
}

// CSV 对齐：列按最大宽度对齐，CJK 宽度参与计算。
func TestStructuredCSVAlign(t *testing.T) {
	p := writeTmp(t, "a.csv", "name,备注,age\nalice,你好,30\nbob,世界,7\n")
	ren, err := Render(p, 100, 24, ProtocolNone, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(stripANSIRaw(ren.Content), "\n")
	// 数据行的最后一列起始位置应一致：alice 行的 "30" 与 bob 行的 "7"
	var ageCols []int
	for _, l := range lines {
		if strings.Contains(l, "alice") {
			ageCols = append(ageCols, strings.Index(l, "30"))
		}
		if strings.Contains(l, "bob") {
			ageCols = append(ageCols, strings.Index(l, "bob    世界  7")+len("bob    世界  "))
		}
	}
	if len(ageCols) != 2 || ageCols[0] != ageCols[1] {
		t.Errorf("age 列未对齐: %v\n%s", ageCols, ren.Content)
	}
	if ren.Lang != "CSV" {
		t.Errorf("Lang = %q", ren.Lang)
	}
}

// TSV 走制表符分隔。
func TestStructuredTSV(t *testing.T) {
	p := writeTmp(t, "a.tsv", "x\ty\n1\t2\n")
	ren, err := Render(p, 60, 24, ProtocolNone, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ren.Content, "1") || !strings.Contains(ren.Content, "2") {
		t.Errorf("TSV 渲染异常:\n%s", ren.Content)
	}
}

func stripANSIRaw(s string) string {
	// 复用 code_test 的 ansiRe 不便跨文件，测试内联简化版
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEsc = true
		case inEsc && (r == 'm' || r == 'K'):
			inEsc = false
		case !inEsc:
			b.WriteRune(r)
		}
	}
	return b.String()
}
