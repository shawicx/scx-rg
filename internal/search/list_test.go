package search

import (
	"context"
	"testing"
)

func TestListProviderFiltersAndSorts(t *testing.T) {
	p := ListProvider{Candidates: []Candidate{
		{Text: "alpha.txt", Detail: "d1"},
		{Text: "beta.log"},
		{Text: "gamma-alpha.md"},
	}}
	rs, err := p.Search(context.Background(), "", "alp")
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("命中 %d 条, 期望 2: %v", len(rs), rs)
	}
	// 词首连续命中的 alpha.txt 应排前（边界 + 连续加权）
	if rs[0].Path != "alpha.txt" {
		t.Errorf("首位 = %q, 期望 alpha.txt（词首连续命中优先）", rs[0].Path)
	}
	if rs[0].Text != rs[0].Path {
		t.Errorf("Text 应等于候选行（PickLine 输出依赖）: %q vs %q", rs[0].Text, rs[0].Path)
	}
	if rs[0].Detail != "d1" {
		t.Errorf("Detail 未透传: %q", rs[0].Detail)
	}
}

func TestListProviderEmptyQueryListsAll(t *testing.T) {
	p := ListProvider{Candidates: []Candidate{{Text: "a"}, {Text: "b"}, {Text: "c"}}}
	rs, err := p.Search(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 3 {
		t.Fatalf("空查询应全量列出, 实际 %d 条", len(rs))
	}
}
