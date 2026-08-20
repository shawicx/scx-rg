// Package search 提供搜索后端抽象与实现。
package search

import "context"

// MaxResults 单次搜索返回上限。
const MaxResults = 500

// Result 一条搜索结果。
// Line 为 0 表示文件名匹配（files 模式），否则为内容匹配行号（content 模式）。
// Hits 为文件名模式下的模糊命中位置（相对 Path 的 rune 下标），用于列表高亮。
// Err 非空时表示搜索本身失败（如非法正则），应终止消费并展示错误。
type Result struct {
	Path string
	Line int
	Text string
	Hits []int
	Err  error
}

// Provider 搜索后端抽象。
type Provider interface {
	Name() string
}

// SyncProvider 一次性返回全部结果的同步后端（如文件名搜索）。
type SyncProvider interface {
	Provider
	Search(ctx context.Context, root, query string) ([]Result, error)
}

// StreamProvider 边搜边出的流式后端（如 rg 内容搜索）。
// channel 在结束/取消/出错后关闭；取消 ctx 应 promptly 终止底层进程。
type StreamProvider interface {
	Provider
	SearchStream(ctx context.Context, root, query string) (<-chan Result, error)
}
