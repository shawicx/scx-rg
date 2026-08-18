// Package search 提供搜索后端抽象与实现。
package search

import "context"

// Result 一条搜索结果。
// Line 为 0 表示文件名匹配（files 模式），否则为内容匹配行号（content 模式）。
type Result struct {
	Path string
	Line int
	Text string
}

// Provider 搜索后端抽象，后续可替换为纯 Go 实现、stdin 候选列表等。
type Provider interface {
	Name() string
	Search(ctx context.Context, root, query string) ([]Result, error)
}
