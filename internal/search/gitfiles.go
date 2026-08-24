package search

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

// GitBlameFile 拉取整份行级 blame（--line-porcelain），返回原始输出，
// 解析交给 ParseBlamePorcelain（纯函数便于测试）。非 git 仓库返回错误。
func GitBlameFile(ctx context.Context, run GitRunner, root, relPath string) (string, error) {
	if run == nil {
		run = realGitRunner(root)
	}
	out, err := run(ctx, "blame", "--line-porcelain", "--", relPath)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// BlameLine 一行的 blame 摘要要素。
type BlameLine struct {
	Hash   string
	Author string
	Time   time.Time
}

// ParseBlamePorcelain 解析 git blame --line-porcelain 输出为
// 行号(1 起) → "短hash 作者 相对时间" 摘要表。
// porcelain 格式：每个 chunk 以 "hash origLine finalLine numLines" 头开始，
// 后跟 header 行（author / author-time / summary 等），以 "\t正文" 结束。
func ParseBlamePorcelain(raw string) map[int]string {
	now := time.Now()
	out := map[int]string{}
	var (
		cur    BlameLine
		lineNo int
		inHead bool
	)
	for _, l := range strings.Split(raw, "\n") {
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "\t") {
			lineNo++
			out[lineNo] = blameSummary(cur, now)
			inHead = false
			continue
		}
		// chunk 头：hash 起始（40 位 hex）后跟行号三元组
		fields := strings.Fields(l)
		if len(fields) == 4 && len(fields[0]) == 40 && isAllHex(fields[0]) {
			cur = BlameLine{Hash: fields[0][:7]}
			lineNo, _ = strconv.Atoi(fields[1])
			lineNo-- // 正文行自增
			inHead = true
			continue
		}
		if !inHead {
			continue
		}
		switch {
		case strings.HasPrefix(l, "author "):
			cur.Author = clipAuthor(strings.TrimPrefix(l, "author "))
		case strings.HasPrefix(l, "author-time "):
			if ts, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(l, "author-time ")), 10, 64); err == nil {
				cur.Time = time.Unix(ts, 0)
			}
		}
	}
	return out
}

func isAllHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// clipAuthor 截长作者名（状态栏宽度有限）。
func clipAuthor(a string) string {
	if i := strings.IndexByte(a, ' '); i > 0 { // 去 "A U Thor" 里的中间名尾巴
		// git 常见格式 "A U Thor"——保留前两段拼成缩写风格
		if seg := strings.Fields(a); len(seg) >= 2 {
			return seg[0] + " " + seg[len(seg)-1]
		}
	}
	if len(a) > 16 {
		return a[:16]
	}
	return a
}

// blameSummary "短hash 作者 相对时间/日期"。超过 30 天显示绝对日期
// （旧提交的相对天数既难读也会让快照测试逐日漂移）。
func blameSummary(b BlameLine, now time.Time) string {
	age := now.Sub(b.Time)
	rel := relativeTime(age)
	if age > 30*24*time.Hour {
		rel = b.Time.Format("2006-01-02")
	}
	if b.Author == "" {
		return b.Hash + " " + rel
	}
	return b.Hash + " " + b.Author + " " + rel
}

// relativeTime 紧凑相对时间（3d / 2h / 5m）。
func relativeTime(d time.Duration) string {
	switch {
	case d < 0:
		d = 0
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	default:
		return strconv.Itoa(int(d/(24*time.Hour))) + "d"
	}
	return ""
}

// GitShowCommit 取一个 commit 的详情（--stat + 完整 message），供详情面板。
func GitShowCommit(ctx context.Context, run GitRunner, root, hash string) (string, error) {
	if run == nil {
		run = realGitRunner(root)
	}
	out, err := run(ctx, "show", "--stat", "--format=fuller", hash)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
