package search

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// FilesProvider 文件名搜索。
// UseRg 为 true 时用 rg --files 枚举（尊重 .gitignore / 排除隐藏文件），
// 否则回退到内置目录遍历（跳过常见依赖/构建目录）。
// Exact 为 true 时要求分词以完整子串形式命中（Ctrl+F 切换），否则用模糊
// 子序列匹配；模糊模式下散落拼凑的低质量匹配会被过滤（宁缺毋滥）。
// IgnoreExtra 为额外忽略的目录名（来自 config），两条枚举路径都生效。
type FilesProvider struct {
	UseRg       bool
	Exact       bool
	IgnoreExtra []string
}

func (FilesProvider) Name() string { return "files" }

func (p FilesProvider) Search(ctx context.Context, root, query string) ([]Result, error) {
	var candidates []Candidate
	var err error
	if p.UseRg {
		candidates, err = ListFiles(ctx, root, p.IgnoreExtra)
	} else {
		candidates, err = walkFiles(ctx, root, p.IgnoreExtra)
	}
	if err != nil {
		return nil, err
	}
	return matchCandidates(candidates, query, p.Exact), nil
}

// matchCandidates 对候选列表做模糊/精确打分排序，返回最多 MaxResults 条。
// files 模式与 finder（stdin / docker-ps 候选）共用同一套匹配语义。
func matchCandidates(candidates []Candidate, query string, exact bool) []Result {
	type scored struct {
		r     Result
		score int
	}
	needSort := strings.Fields(query) != nil
	var out []scored
	for _, c := range candidates {
		var m FuzzyMatch
		if exact {
			m = ExactMatch(query, c.Text)
		} else {
			m = Fuzzy(query, c.Text)
		}
		if !m.Matched || m.Scattered {
			continue
		}
		out = append(out, scored{r: Result{Path: c.Text, Text: c.Text, Hits: m.Positions, Detail: c.Detail}, score: m.Score})
		if !needSort && len(out) >= MaxResults {
			break // 空查询不排序，直接截断
		}
	}
	if needSort {
		sort.Slice(out, func(i, j int) bool {
			if out[i].score != out[j].score {
				return out[i].score > out[j].score
			}
			return out[i].r.Path < out[j].r.Path
		})
		if len(out) > MaxResults {
			out = out[:MaxResults]
		}
	}
	results := make([]Result, len(out))
	for i, s := range out {
		results[i] = s.r
	}
	return results
}

// ListFiles 用 rg --files 枚举文件（自动尊重 .gitignore 与隐藏文件规则）。
// ignore 为额外排除的目录名，转成 rg -g '!name/' glob。
func ListFiles(ctx context.Context, root string, ignore []string) ([]Candidate, error) {
	args := make([]string, 0, 2+2*len(ignore))
	args = append(args, "--files")
	for _, d := range ignore {
		args = append(args, "-g", "!"+d+"/")
	}
	args = append(args, ".")
	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = root
	raw, err := cmd.Output()
	if err != nil && len(raw) == 0 {
		return nil, fmt.Errorf("rg --files 失败: %w", err)
	}
	// rg 退出码 2 表示遍历中遇到过错误（macOS 隐私保护目录报
	// Operation not permitted 等）；stdout 已有的枚举结果仍然有效，
	// 只有空输出才视为整体失败。
	var files []Candidate
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		line = strings.TrimPrefix(line, "./")
		if line == "" {
			continue
		}
		files = append(files, Candidate{Text: line})
		if len(files) >= MaxResults {
			break
		}
	}
	return files, nil
}

var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, "node_modules": true,
	"target": true, "dist": true, ".next": true, ".idea": true,
	".vscode": true, "vendor": true, "__pycache__": true, ".cache": true,
}

var errStopWalk = errors.New("stop walk")

// walkFiles 内置目录遍历兜底（rg 不可用时使用）。
func walkFiles(ctx context.Context, root string, ignore []string) ([]Candidate, error) {
	extra := make(map[string]bool, len(ignore))
	for _, d := range ignore {
		extra[d] = true
	}
	var out []Candidate
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 无法访问的路径直接跳过
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (skipDirs[name] || extra[name] || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || strings.HasPrefix(name, ".") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		out = append(out, Candidate{Text: rel})
		if len(out) >= MaxResults {
			return errStopWalk
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, errStopWalk) && ctx.Err() == nil {
		return nil, walkErr
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Text < out[j].Text })
	return out, nil
}
