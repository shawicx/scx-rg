package search

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// MaxResults 单次搜索返回上限。
const MaxResults = 500

var errStopWalk = errors.New("stop walk")

// FilesProvider 按文件名子串匹配（忽略大小写）；空查询列出全部文件。
type FilesProvider struct{}

func (FilesProvider) Name() string { return "files" }

var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, "node_modules": true,
	"target": true, "dist": true, ".next": true, ".idea": true,
	".vscode": true, "vendor": true, "__pycache__": true, ".cache": true,
}

func (p FilesProvider) Search(ctx context.Context, root, query string) ([]Result, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []Result
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
		if q != "" && !strings.Contains(strings.ToLower(rel), q) {
			return nil
		}
		out = append(out, Result{Path: rel})
		if len(out) >= MaxResults {
			return errStopWalk
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, errStopWalk) && ctx.Err() == nil {
		return nil, walkErr
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
