package search

import (
	"sort"
	"strings"
	"unicode"
)

// FuzzyMatch 模糊匹配结果。
type FuzzyMatch struct {
	Matched   bool
	Score     int
	Positions []int // 候选串上的命中位置（rune 下标），升序去重
	// Scattered 为 true 表示低质量「散落拼凑」匹配：既无完整子串命中、也非
	// 边界缩写、命中跨度又过大。由调用方决定丢弃（宁缺毋滥）。
	Scattered bool
}

// Fuzzy 判断 candidate 是否命中 query：
// query 按空白分词，每个词都必须是 candidate（忽略大小写）的有序子序列（AND 语义）。
// 评分分层：完整子串命中（+50，边界起点再 +10）远高于边界/连续命中（各 +8）；
// 命中字符间的跨度越大越降分。
func Fuzzy(query, candidate string) FuzzyMatch {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return FuzzyMatch{Matched: true}
	}
	runes := []rune(candidate)
	lower := []rune(strings.ToLower(candidate))

	seen := make(map[int]bool, len(runes))
	score := 0
	scattered := false
	for _, term := range terms {
		pos, s, q := matchTerm([]rune(strings.ToLower(term)), runes, lower)
		if pos == nil {
			return FuzzyMatch{}
		}
		for _, p := range pos {
			seen[p] = true
		}
		score += s
		if q.junk {
			scattered = true
		}
	}

	positions := make([]int, 0, len(seen))
	for p := range seen {
		positions = append(positions, p)
	}
	sort.Ints(positions)
	score -= len(runes) - len(positions) // 未命中的字符越多，惩罚越重
	return FuzzyMatch{Matched: true, Score: score, Positions: positions, Scattered: scattered}
}

// ExactMatch 精确（子串）匹配：query 按空白分词，每个词都必须以忽略大小写的
// 子串形式出现在 candidate 中；返回全部出现位置用于高亮。命中在文件名内比
// 命中在目录名里加分。
func ExactMatch(query, candidate string) FuzzyMatch {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return FuzzyMatch{Matched: true}
	}
	runes := []rune(candidate)
	lower := []rune(strings.ToLower(candidate))
	baseStart := 0
	if i := strings.LastIndex(candidate, "/"); i >= 0 {
		baseStart = len([]rune(candidate[:i+1]))
	}

	seen := make(map[int]bool, len(runes))
	score := 0
	for _, term := range terms {
		t := []rune(strings.ToLower(term))
		occurs := allOccurrences(lower, t)
		if len(occurs) == 0 {
			return FuzzyMatch{}
		}
		inBase := false
		for _, i := range occurs {
			for j := range t {
				seen[i+j] = true
			}
			if i >= baseStart {
				inBase = true
			}
		}
		score += 50
		if inBase {
			score += 10
		}
	}

	positions := make([]int, 0, len(seen))
	for p := range seen {
		positions = append(positions, p)
	}
	sort.Ints(positions)
	score -= len(runes) - len(positions)
	return FuzzyMatch{Matched: true, Score: score, Positions: positions}
}

// termQuality 单个词的匹配质量：substr=完整子串命中；acronym=全部命中都在
// 边界（首字母缩写式）；junk=散落拼凑（无子串、非缩写、跨度松散）。
type termQuality struct{ substr, acronym, junk bool }

// matchTerm 在候选中自左向右贪心匹配一个词的子序列，返回命中位置与得分。
func matchTerm(term, orig, lower []rune) ([]int, int, termQuality) {
	// 完整子串命中：最高层
	if idx := indexRunes(lower, term); idx >= 0 {
		positions := make([]int, len(term))
		for i := range positions {
			positions[i] = idx + i
		}
		score := 50
		if isBoundary(orig, idx) {
			score += 10
		}
		return positions, score, termQuality{substr: true}
	}

	positions := make([]int, 0, len(term))
	score := 0
	allBoundary := true
	for _, tc := range term {
		found := -1
		start := 0
		if n := len(positions); n > 0 {
			start = positions[n-1] + 1 // 保持有序子序列
		}
		for i := start; i < len(lower); i++ {
			if lower[i] == tc {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, 0, termQuality{}
		}
		s := 1
		if isBoundary(orig, found) {
			s += 8
		} else {
			allBoundary = false
		}
		if n := len(positions); n > 0 && found == positions[n-1]+1 {
			s += 8
		}
		score += s
		positions = append(positions, found)
	}
	span := positions[len(positions)-1] - positions[0] + 1
	gap := span - len(term)
	score -= gap // 命中字符之间跳过的字符越多，越降分
	junk := !allBoundary && gap > len(term)+2
	return positions, score, termQuality{acronym: allBoundary, junk: junk}
}

func isBoundary(orig []rune, i int) bool {
	if i == 0 {
		return true
	}
	prev, cur := orig[i-1], orig[i]
	if strings.ContainsRune("/-_./\\ \t", prev) {
		return true
	}
	return unicode.IsLower(prev) && unicode.IsUpper(cur) // 驼峰
}

// indexRunes 返回 needle 在 hay 中的首个 rune 级子串位置，未找到返回 -1。
func indexRunes(hay, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(hay) {
		return -1
	}
outer:
	for i := 0; i+len(needle) <= len(hay); i++ {
		for j, n := range needle {
			if hay[i+j] != n {
				continue outer
			}
		}
		return i
	}
	return -1
}

// allOccurrences 返回 needle 在 hay 中所有出现的起始位置（升序）。
func allOccurrences(hay, needle []rune) []int {
	var out []int
	if len(needle) == 0 {
		return nil
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		match := true
		for j, n := range needle {
			if hay[i+j] != n {
				match = false
				break
			}
		}
		if match {
			out = append(out, i)
			i += len(needle) - 1
		}
	}
	return out
}
