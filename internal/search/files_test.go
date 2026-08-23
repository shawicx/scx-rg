package search

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// writeTree 建临时文件树；git=true 时初始化为 git 仓库（rg 只在 git 仓库内应用 .gitignore）。
func writeTree(t *testing.T, files map[string]string, git bool) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if git {
		if err := exec.Command("git", "init", dir).Run(); err != nil {
			t.Skipf("git 不可用: %v", err)
		}
	}
	return dir
}

func paths(rs []Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Path
	}
	return out
}

func TestListFilesRespectsGitignoreAndHidden(t *testing.T) {
	if !RgAvailable() {
		t.Skip("rg 未安装")
	}
	dir := writeTree(t, map[string]string{
		"a.go":              "",
		"sub/b.go":          "",
		".gitignore":        "sub/\nnode_modules/\n",
		".hidden.txt":       "",
		"node_modules/x.js": "",
	}, true)
	got, err := ListFiles(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.go"}; !reflect.DeepEqual(candidateTexts(got), want) {
		t.Fatalf("rg --files 枚举 = %v, 期望 %v（应尊重 .gitignore 且排除隐藏目录）", candidateTexts(got), want)
	}
}

// candidateTexts 提取候选文本，便于与字符串切片断言。
func candidateTexts(cs []Candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Text
	}
	return out
}

// rg 遍历中遇到个别不可读目录（macOS 隐私保护目录报 Operation not permitted）
// 会以退出码 2 结束，但 stdout 已有的枚举结果仍然有效，不应整体失败
// （2026-08-22 修复：在 ~ 目录下整帧报「rg --files 失败: exit status 2」）。
func TestListFilesToleratesUnreadableSubdir(t *testing.T) {
	if !RgAvailable() {
		t.Skip("rg 未安装")
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("Windows / root 环境下 chmod 000 不构成读障碍")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755) //nolint:errcheck // 恢复权限以便 t.TempDir 清理

	got, err := ListFiles(context.Background(), dir, nil)
	if err != nil {
		t.Fatalf("个别目录不可读不应整体失败: %v", err)
	}
	if want := []string{"a.txt"}; !reflect.DeepEqual(candidateTexts(got), want) {
		t.Fatalf("枚举 = %v, 期望 %v", candidateTexts(got), want)
	}
}

func TestFilesProviderFuzzyOrdersByScore(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.go":           "",
		"my-great-model.go": "",
		"zzz.txt":           "",
	}, false)
	rs, err := (FilesProvider{}).Search(context.Background(), dir, "mg")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"main.go", "my-great-model.go"}; !reflect.DeepEqual(paths(rs), want) {
		t.Fatalf("模糊排序结果 = %v, 期望 %v（按评分降序，不匹配的剔除）", paths(rs), want)
	}
	if r := rs[0]; !reflect.DeepEqual(r.Hits, []int{0, 5}) {
		t.Fatalf("main.go 的命中位置 = %v, 期望 [0 5]", r.Hits)
	}
}

func TestFilesProviderEmptyQueryListsAllSorted(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"b.txt": "",
		"a.go":  "",
		"c.md":  "",
	}, false)
	rs, err := (FilesProvider{}).Search(context.Background(), dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.go", "b.txt", "c.md"}; !reflect.DeepEqual(paths(rs), want) {
		t.Fatalf("空查询结果 = %v, 期望按路径全量列出 %v", paths(rs), want)
	}
}

func TestFilesProviderRespectsSkipDirs(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"ok.go":               "",
		"node_modules/bad.js": "",
		".git/hidden":         "",
	}, false)
	rs, err := (FilesProvider{UseRg: false}).Search(context.Background(), dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"ok.go"}; !reflect.DeepEqual(paths(rs), want) {
		t.Fatalf("walk 回退应跳过 node_modules/.git，结果 = %v 期望 %v", paths(rs), want)
	}
}

func TestFilesProviderRgEnumerationSameBehavior(t *testing.T) {
	if !RgAvailable() {
		t.Skip("rg 未安装")
	}
	dir := writeTree(t, map[string]string{
		"main.go":           "",
		"my-great-model.go": "",
		"sub/ignored.go":    "",
		".gitignore":        "sub/\n",
	}, true)
	rs, err := (FilesProvider{UseRg: true}).Search(context.Background(), dir, "model")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"my-great-model.go"}; !reflect.DeepEqual(paths(rs), want) {
		t.Fatalf("rg 枚举 + 模糊过滤 = %v, 期望 %v", paths(rs), want)
	}
}

func TestFilesProviderFiltersScatteredJunk(t *testing.T) {
	// 用户实测案例：搜 clear，目录字母拼凑出的远距离散匹配应被过滤
	dir := writeTree(t, map[string]string{
		"scripts/clear.sh": "",
		"alibabacloud/hbrclient/c/job-0000418crpa026lfsifr_0.csv": "",
	}, false)
	rs, err := FilesProvider{UseRg: false}.Search(context.Background(), dir, "clear")
	if err != nil {
		t.Fatal(err)
	}
	got := paths(rs)
	if !reflect.DeepEqual(got, []string{"scripts/clear.sh"}) {
		t.Fatalf("散落拼凑应被过滤，结果 = %v", got)
	}
}

func TestFilesProviderExactMode(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"scripts/clear.sh":  "",
		"my-great-model.go": "",
		"great.txt":         "",
		"agrtz.log":         "",
		"alibabacloud/hbrclient/c/job-0000418crpa026lfsifr_0.csv": "",
	}, false)
	ctx := context.Background()

	// 模糊：紧凑子序列均命中
	fuzzy, err := FilesProvider{UseRg: false}.Search(ctx, dir, "grt")
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(fuzzy); len(got) != 3 { // my-great-model.go / great.txt / agrtz.log
		t.Fatalf("模糊模式应命中 3 个紧凑匹配，得到 %v", got)
	}

	// 精确：只有完整子串
	exact, err := FilesProvider{UseRg: false, Exact: true}.Search(ctx, dir, "grt")
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(exact); !reflect.DeepEqual(got, []string{"agrtz.log"}) {
		t.Fatalf("精确模式只应保留子串命中，得到 %v", got)
	}
}
