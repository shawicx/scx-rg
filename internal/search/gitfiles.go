package search

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitRunner 可注入的 git 执行器（测试换 fake）；所有命令以 -C root 执行。
type GitRunner func(ctx context.Context, args ...string) ([]byte, error)

// realGitRunner 真实 git 子进程执行器。
func realGitRunner(root string) GitRunner {
	return func(ctx context.Context, args ...string) ([]byte, error) {
		full := append([]string{"-C", root}, args...)
		cmd := exec.CommandContext(ctx, "git", full...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return out, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, nil
	}
}

// GitChangedFiles 返回相对 root 的变更文件列表（仅 tracked 文件，不含未跟踪新文件）。
// staged=false 为对 HEAD 的全部变更（暂存+未暂存）；staged=true 仅已暂存部分。
// root 不在 git 仓库内或没有 git 二进制时返回错误，调用方据此隐藏 Git 筛选。
func GitChangedFiles(ctx context.Context, run GitRunner, root string, staged bool) ([]string, error) {
	if run == nil {
		run = realGitRunner(root)
	}
	top, err := run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	repoRoot := filepath.Clean(strings.TrimSpace(string(top)))
	args := []string{"diff", "--name-only"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "HEAD")
	out, err := run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return relativizeGitPaths(strings.Split(strings.TrimRight(string(out), "\n"), "\n"), repoRoot, root), nil
}

// relativizeGitPaths 把仓库根相对路径换算成搜索根相对路径；搜索根是仓库
// 子目录时丢弃范围外的文件并剥离前缀。目录统一用 / 分隔（git 输出风格）。
func relativizeGitPaths(lines []string, repoRoot, root string) []string {
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rel, err := filepath.Rel(root, filepath.Join(repoRoot, line))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue // 搜索根之外
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}
