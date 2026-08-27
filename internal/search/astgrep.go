package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// ast-grep 集成：AST 模式匹配 + 重写。ast-grep 结果与 rg 结果是两套
// 语义，作为独立能力挂接（不依附文件名/内容列表）。安全模型：进入替换
// 前要求 git 仓库且工作区干净（改动审查与回滚交给 git），应用编辑由本包
// 完成（字节区间拼接，不依赖 ast-grep 的写文件行为）。

// AstMatch 一处 AST 匹配：文件、起始行（1 起）、命中文本、重写文本与
// 字节区间（应用于同一文件的多处匹配按区间倒序拼接）。
type AstMatch struct {
	File        string
	Line        int
	Text        string
	Replacement string
	Start       int
	End         int
}

// AstGrepAvailable 探测 ast-grep 二进制。
func AstGrepAvailable() bool {
	_, err := exec.LookPath("ast-grep")
	return err == nil
}

// astEntry ast-grep --json 输出的一条记录（line/column 为 0 起）。
type astEntry struct {
	Text        string `json:"text"`
	File        string `json:"file"`
	Replacement string `json:"replacement"`
	Range       struct {
		Start struct {
			Line   int  `json:"line"`
			Column int  `json:"column"`
			Offset *int `json:"byteOffset"`
		} `json:"start"`
		End struct {
			Line   int  `json:"line"`
			Column int  `json:"column"`
			Offset *int `json:"byteOffset"`
		} `json:"end"`
	} `json:"range"`
}

// realAstRunner 真实 ast-grep 子进程执行器（cwd=root，ctx 感知）。
func realAstRunner(root string) GitRunner {
	return func(ctx context.Context, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "ast-grep", args...)
		cmd.Dir = root
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return out, fmt.Errorf("ast-grep %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, nil
	}
}

// AstGrepScan 运行 ast-grep run --pattern --rewrite --json，解析匹配并
// 计算字节区间（优先用输出的 byteOffset，缺失时按行/列换算）。
func AstGrepScan(ctx context.Context, run GitRunner, root, pattern, rewrite string) ([]AstMatch, error) {
	if strings.TrimSpace(pattern) == "" {
		return nil, fmt.Errorf("AST 模式不能为空")
	}
	if run == nil {
		run = realAstRunner(root)
	}
	out, err := run(ctx, "run", "--pattern", pattern, "--rewrite", rewrite, "--json", root)
	if err != nil {
		return nil, fmt.Errorf("ast-grep 执行失败: %w", err)
	}
	entries, err := parseAstJSON(out)
	if err != nil {
		return nil, err
	}
	matches := make([]AstMatch, 0, len(entries))
	for _, e := range entries {
		if e.File == "" {
			continue
		}
		file := e.File
		if !strings.HasPrefix(file, "/") {
			file = root + "/" + file
		}
		raw, rerr := os.ReadFile(file)
		if rerr != nil {
			continue
		}
		start, end := astRange(e, raw)
		if start < 0 || end < start || end > len(raw) {
			continue
		}
		text := e.Text
		if text == "" {
			text = string(raw[start:end])
		}
		matches = append(matches, AstMatch{
			File: file, Line: e.Range.Start.Line + 1, Text: text,
			Replacement: e.Replacement, Start: start, End: end,
		})
	}
	return matches, nil
}

func parseAstJSON(raw []byte) ([]astEntry, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, nil
	}
	var entries []astEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("ast-grep 输出解析失败: %w", err)
	}
	return entries, nil
}

// astRange 计算匹配的字节区间：输出带 byteOffset 用之，否则按 0 起的
// 行/列从文件内容换算。
func astRange(e astEntry, raw []byte) (int, int) {
	if e.Range.Start.Offset != nil && e.Range.End.Offset != nil {
		return *e.Range.Start.Offset, *e.Range.End.Offset
	}
	lineOffs := astLineOffsets(raw)
	at := func(line, col int) int {
		if line < 0 || line >= len(lineOffs) {
			return -1
		}
		off := lineOffs[line] + col
		if off > len(raw) {
			return -1
		}
		return off
	}
	return at(e.Range.Start.Line, e.Range.Start.Column), at(e.Range.End.Line, e.Range.End.Column)
}

func astLineOffsets(raw []byte) []int {
	offs := []int{0}
	for i, b := range raw {
		if b == '\n' {
			offs = append(offs, i+1)
		}
	}
	return offs
}

// ApplyAstMatches 把匹配的重写应用到文件（同文件多处按区间倒序拼接，
// 互不重叠时安全；重叠匹配会被 ast-grep 去重，这里再做防御性跳过）。
// 返回被修改的文件数。
func ApplyAstMatches(matches []AstMatch) (int, error) {
	byFile := map[string][]AstMatch{}
	for _, m := range matches {
		byFile[m.File] = append(byFile[m.File], m)
	}
	changed := 0
	for file, ms := range byFile {
		raw, err := os.ReadFile(file)
		if err != nil {
			return changed, err
		}
		sort.Slice(ms, func(i, j int) bool { return ms[i].Start < ms[j].Start })
		var parts []string
		prev := 0
		applied := 0
		for _, m := range ms {
			if m.Start < prev || m.End < m.Start || m.End > len(raw) { // 重叠/越界防御
				continue
			}
			parts = append(parts, string(raw[prev:m.Start]), m.Replacement)
			prev = m.End
			applied++
		}
		if applied == 0 {
			continue
		}
		parts = append(parts, string(raw[prev:]))
		if err := os.WriteFile(file, []byte(strings.Join(parts, "")), 0o644); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}

// GitWorktreeClean 工作区是否干净（无任何未提交改动）。
func GitWorktreeClean(ctx context.Context, run GitRunner, root string) (bool, error) {
	if run == nil {
		run = realGitRunner(root)
	}
	out, err := run(ctx, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) == 0, nil
}
