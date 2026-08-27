package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const astJSONSample = `[
  {"text":"console.log($$$)","file":"a.js",
   "range":{"start":{"line":2,"column":0,"byteOffset":21},"end":{"line":2,"column":17,"byteOffset":38}},
   "replacement":"logger.debug($$$)"}
]`

func TestParseAstJSON(t *testing.T) {
	entries, err := parseAstJSON([]byte(astJSONSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].File != "a.js" || entries[0].Replacement != "logger.debug($$$)" {
		t.Fatalf("解析错误: %+v", entries)
	}
	if entries[0].Range.Start.Offset == nil || *entries[0].Range.Start.Offset != 21 {
		t.Errorf("byteOffset 解析错误")
	}
	if _, err := parseAstJSON([]byte("not-json")); err == nil {
		t.Error("坏 JSON 应报错")
	}
	if e, err := parseAstJSON(nil); err != nil || e != nil {
		t.Error("空输出应为空结果")
	}
}

// astRange：无 byteOffset 时按 0 起行/列换算。
func TestAstRangeFromLineCol(t *testing.T) {
	raw := []byte("line0\nabcdef\nline2\n")
	var e astEntry
	e.Range.Start.Line, e.Range.Start.Column = 1, 2
	e.Range.End.Line, e.Range.End.Column = 1, 5
	start, end := astRange(e, raw)
	if start != 8 || end != 11 {
		t.Errorf("区间 = [%d,%d), want [8,11)", start, end)
	}
}

func TestAstGrepScanRunnerInjection(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.js"), []byte("var x = 1;\n  console.log(x);\nvar y = 2;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	matches, err := AstGrepScan(context.Background(), func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] != "run" || args[1] != "--pattern" {
			t.Errorf("参数不符合 ast-grep run: %v", args)
		}
		fixed := strings.ReplaceAll(astJSONSample, "\"byteOffset\":21", "\"byteOffset\":0")
		fixed = strings.ReplaceAll(fixed, "\"byteOffset\":38", "\"byteOffset\":17")
		return []byte(fixed), nil
	}, dir, "console.log($$$)", "logger.debug($$$)")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("应解析 1 处匹配: %+v", matches)
	}
	m := matches[0]
	if !strings.HasSuffix(m.File, "a.js") || m.Line != 3 || m.Start != 0 || m.End != 17 {
		t.Errorf("匹配 = %+v", m)
	}
	if m.Text != "console.log($$$)" || m.Replacement != "logger.debug($$$)" {
		t.Errorf("文本/重写 = %q / %q", m.Text, m.Replacement)
	}
}

// ApplyAstMatches：同文件多处非重叠匹配按倒序拼接全部生效；越界防御跳过。
func TestApplyAstMatches(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("AA BB CC"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := ApplyAstMatches([]AstMatch{
		{File: p, Start: 0, End: 2, Replacement: "xx"}, // AA → xx
		{File: p, Start: 6, End: 8, Replacement: "zz"}, // CC → zz
		{File: p, Start: 7, End: 9, Replacement: "!!"}, // 越界/重叠 → 跳过
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("应修改 1 个文件: %d", n)
	}
	raw, _ := os.ReadFile(p)
	if string(raw) != "xx BB zz" {
		t.Errorf("应用结果 = %q", string(raw))
	}
}

func TestGitWorktreeClean(t *testing.T) {
	clean, _ := GitWorktreeClean(context.Background(), func(context.Context, ...string) ([]byte, error) {
		return nil, nil // porcelain 无输出 = 干净
	}, "/repo")
	if !clean {
		t.Error("空 porcelain 输出应为干净")
	}
	dirty, _ := GitWorktreeClean(context.Background(), func(context.Context, ...string) ([]byte, error) {
		return []byte(" M a.go\n"), nil
	}, "/repo")
	if dirty {
		t.Error("有改动应为不干净")
	}
}
