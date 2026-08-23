package search

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGit 按子命令返回固定输出，并记录调用参数。
type fakeGit struct {
	repoRoot string
	diff     string
	diffErr  error
	calls    [][]string
}

func (f *fakeGit) run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	switch args[0] {
	case "rev-parse":
		if f.repoRoot == "" {
			return nil, errors.New("fatal: not a git repository")
		}
		return []byte(f.repoRoot + "\n"), nil
	case "diff":
		if f.diffErr != nil {
			return nil, f.diffErr
		}
		return []byte(f.diff), nil
	}
	return nil, errors.New("unexpected git call")
}

func TestGitChangedFilesSubdirRelativize(t *testing.T) {
	f := &fakeGit{repoRoot: "/repo", diff: "a.go\nsub/b.go\noutside/c.go\n\n"}
	files, err := GitChangedFiles(context.Background(), f.run, "/repo/sub", false)
	if err != nil {
		t.Fatal(err)
	}
	// 搜索根 /repo/sub：b.go 保留，a.go/outside 丢弃
	if got := strings.Join(files, ","); got != "b.go" {
		t.Errorf("files = %q, want [b.go]", got)
	}
}

func TestGitChangedFilesRepoRootEqualsSearchRoot(t *testing.T) {
	f := &fakeGit{repoRoot: "/repo", diff: "a.go\nsub/b.go\n"}
	files, err := GitChangedFiles(context.Background(), f.run, "/repo", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(files, ","); got != "a.go,sub/b.go" {
		t.Errorf("files = %q, want [a.go sub/b.go]", got)
	}
}

func TestGitChangedFilesStagedFlag(t *testing.T) {
	f := &fakeGit{repoRoot: "/repo", diff: "a.go\n"}
	if _, err := GitChangedFiles(context.Background(), f.run, "/repo", true); err != nil {
		t.Fatal(err)
	}
	var diffCall []string
	for _, c := range f.calls {
		if c[0] == "diff" {
			diffCall = c
		}
	}
	joined := strings.Join(diffCall, " ")
	if !strings.Contains(joined, "--cached") {
		t.Errorf("staged 应携带 --cached: %q", joined)
	}
	if !strings.Contains(joined, "HEAD") {
		t.Errorf("diff 应对比 HEAD: %q", joined)
	}
}

func TestGitChangedFilesNotARepo(t *testing.T) {
	f := &fakeGit{}
	if _, err := GitChangedFiles(context.Background(), f.run, "/tmp/whatever", false); err == nil {
		t.Error("非 git 仓库应返回错误")
	}
}

func TestRelativizeGitPathsCleanSeparators(t *testing.T) {
	// git 输出恒用 /，Windows 风格 root 也不应产生 \ 路径
	got := relativizeGitPaths([]string{"sub/x.go"}, filepath.FromSlash("/repo"), filepath.FromSlash("/repo"))
	for _, p := range got {
		if strings.ContainsRune(p, '\\') {
			t.Errorf("路径应统一用 / 分隔: %q", p)
		}
	}
}
