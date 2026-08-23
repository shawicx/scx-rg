package search

import "context"

// ListProvider 静态候选列表搜索——finder 模式（--provider stdin / docker-ps）
// 的后端：候选行与文件系统零耦合，模糊打分复用 files 模式同一套语义
// （分词 AND、边界/连续加权、散落噪声过滤）。
type ListProvider struct {
	Candidates []Candidate
	Exact      bool
}

func (ListProvider) Name() string { return "list" }

func (p ListProvider) Search(ctx context.Context, root, query string) ([]Result, error) {
	return matchCandidates(p.Candidates, query, p.Exact), nil
}
