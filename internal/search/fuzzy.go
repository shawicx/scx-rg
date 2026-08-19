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
}

// Fuzzy 判断 candidate 是否命中 query：
// query 按空白分词，每个词都必须是 candidate（忽略大小写）的有序子序列（AND 语义）。
// 评分偏好：边界命中（首字符 / 分隔符后 / 驼峰）、连续命中、未命中字符更少的候选。
func Fuzzy(query, candidate string) FuzzyMatch {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return FuzzyMatch{Matched: true}
	}
	runes := []rune(candidate)
	lower := []rune(strings.ToLower(candidate))

	seen := make(map[int]bool, len(runes))
	score := 0
	for _, term := range terms {
		pos, s := matchTerm([]rune(strings.ToLower(term)), runes, lower)
		if pos == nil {
			return FuzzyMatch{}
		}
		for _, p := range pos {
			seen[p] = true
		}
		score += s
	}

	positions := make([]int, 0, len(seen))
	for p := range seen {
		positions = append(positions, p)
	}
	sort.Ints(positions)
	score -= len(runes) - len(positions) // 未命中的字符越多，惩罚越重
	return FuzzyMatch{Matched: true, Score: score, Positions: positions}
}

// matchTerm 在候选中自左向右贪心匹配一个词的子序列，返回命中位置与得分。
func matchTerm(term, orig, lower []rune) ([]int, int) {
	positions := make([]int, 0, len(term))
	score := 0
	from := 0
	for _, tc := range term {
		found := -1
		for i := from; i < len(lower); i++ {
			if lower[i] == tc {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, 0
		}
		s := 1
		if isBoundary(orig, found) {
			s += 8
		}
		if n := len(positions); n > 0 && found == positions[n-1]+1 {
			s += 8
		}
		score += s
		positions = append(positions, found)
		from = found + 1
	}
	return positions, score
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
