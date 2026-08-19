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

// FilesProvider 文件名模糊搜索。
// UseRg 为 true 时用 rg --files 枚举（尊重 .gitignore / 排除隐藏文件），
// 否则回退到内置目录遍历（跳过常见依赖/构建目录）。
type FilesProvider struct{ UseRg bool }

func (FilesProvider) Name() string { return "files" }

func (p FilesProvider) Search(ctx context.Context, root, query string) ([]Result, error) {
	var candidates []string
	var err error
	if p.UseRg {
		candidates, err = ListFiles(ctx, root)
	} else {
		candidates, err = walkFiles(ctx, root)
	}
	if err != nil {
		return nil, err
	}

	type scored struct {
		r     Result
		score int
	}
	needSort := strings.Fields(query) != nil
	var out []scored
	for _, rel := range candidates {
		m := Fuzzy(query, rel)
		if !m.Matched {
			continue
		}
		out = append(out, scored{r: Result{Path: rel, Hits: m.Positions}, score: m.Score})
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
	return results, nil
}

// ListFiles 用 rg --files 枚举文件（自动尊重 .gitignore 与隐藏文件规则）。
func ListFiles(ctx context.Context, root string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "rg", "--files", ".")
	cmd.Dir = root
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("rg --files 失败: %w", err)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		line = strings.TrimPrefix(line, "./")
		if line == "" {
			continue
		}
		files = append(files, line)
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
func walkFiles(ctx context.Context, root string) ([]string, error) {
	var out []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 无法访问的路径直接跳过
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (skipDirs[name] || strings.HasPrefix(name, ".")) {
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
		out = append(out, rel)
		if len(out) >= MaxResults {
			return errStopWalk
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, errStopWalk) && ctx.Err() == nil {
		return nil, walkErr
	}
	sort.Strings(out)
	return out, nil
}
