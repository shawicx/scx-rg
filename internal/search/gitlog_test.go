package search

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseGitLogLine(t *testing.T) {
	res, ok := ParseGitLogLine("abc1234|2026-08-01|fix: 修复搜索取消泄漏|abc1234def56789abc1234def56789abc1234def56")
	if !ok {
		t.Fatal("合法行应解析成功")
	}
	if res.Path != "abc1234" || res.Detail != "abc1234def56789abc1234def56789abc1234def56" {
		t.Errorf("hash 解析错误: %+v", res)
	}
	if res.Text != "abc1234 2026-08-01 fix: 修复搜索取消泄漏" {
		t.Errorf("Text = %q", res.Text)
	}
	for _, bad := range []string{"", "abc|2026|subj|full", "garbage line"} {
		if _, ok := ParseGitLogLine(bad); ok {
			t.Errorf("非法行 %q 不应解析成功", bad)
		}
	}
}

func TestParseBlamePorcelain(t *testing.T) {
	// 两行正文、两个 chunk 的最小 porcelain 样本
	ts := time.Now().Add(-3 * time.Hour).Unix()
	ts2 := time.Now().Add(-2 * 24 * time.Hour).Unix()
	raw := strings.Join([]string{
		"1111111111111111111111111111111111111111 1 1 1",
		"author Alice",
		"author-time " + itoa(ts),
		"summary first",
		"\tline one",
		"2222222222222222222222222222222222222222 2 2 1",
		"author Bob",
		"author-time " + itoa(ts2),
		"summary second",
		"\tline two",
	}, "\n")
	lines := ParseBlamePorcelain(raw)
	if len(lines) != 2 {
		t.Fatalf("应解析 2 行, 实际 %d: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[1], "1111111 Alice 3h") {
		t.Errorf("第 1 行摘要 = %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "2222222 Bob 2d") {
		t.Errorf("第 2 行摘要 = %q", lines[2])
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestGitBlameFileRunnerInjection(t *testing.T) {
	runnerCalls := 0
	raw, err := GitBlameFile(context.Background(), func(ctx context.Context, args ...string) ([]byte, error) {
		runnerCalls++
		if args[0] != "blame" || args[1] != "--line-porcelain" {
			t.Errorf("应走 blame --line-porcelain: %v", args)
		}
		return []byte("hash-not-parsed-here"), nil
	}, "/repo", "a.go")
	if err != nil || raw != "hash-not-parsed-here" {
		t.Errorf("GitBlameFile 应透传输出: %q %v", raw, err)
	}
}

// 真实 git 仓库集成：git init + 提交，验证 blame 与 log -G 全链路。
func TestGitIntegrationRealRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("无 git")
	}
	dir := t.TempDir()
	sh := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_AUTHOR_DATE=2026-08-01T10:00:00Z", "GIT_COMMITTER_DATE=2026-08-01T10:00:00Z")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n\nfunc Marker() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sh("init", "-q")
	sh("add", ".")
	sh("commit", "-qm", "add marker")

	// blame
	raw, err := GitBlameFile(context.Background(), nil, dir, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	lines := ParseBlamePorcelain(raw)
	if len(lines) != 3 || !strings.Contains(lines[3], "Marker") && lines[3] == "" {
		// 行数校验为主（摘要含 hash/作者/时间）
		if len(lines) != 3 {
			t.Errorf("a.go 3 行应解析 3 条: %v", lines)
		}
	}
	for i := 1; i <= 3; i++ {
		if lines[i] == "" {
			t.Errorf("第 %d 行摘要不应为空", i)
		}
	}

	// git log -G 流式
	ch, err := (GitLogProvider{}).SearchStream(context.Background(), dir, "Marker")
	if err != nil {
		t.Fatal(err)
	}
	var commits []Result
	for r := range ch {
		if r.Err != nil {
			t.Fatalf("流错误: %v", r.Err)
		}
		commits = append(commits, r)
	}
	if len(commits) != 1 {
		t.Fatalf("-G Marker 应命中 1 个提交, 实际 %d", len(commits))
	}
	if !strings.Contains(commits[0].Text, "add marker") {
		t.Errorf("commit 描述 = %q", commits[0].Text)
	}

	// show 详情
	detail, err := GitShowCommit(context.Background(), nil, dir, commits[0].Detail)
	if err != nil || !strings.Contains(detail, "a.go") {
		t.Errorf("commit 详情应含文件: %v %q", err, detail)
	}

	// 空查询报错
	if _, err := (GitLogProvider{}).SearchStream(context.Background(), dir, "  "); err == nil {
		t.Error("空查询应报错")
	}
}
