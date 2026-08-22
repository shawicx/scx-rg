package search

import (
	"reflect"
	"testing"
)

func TestFuzzyEmptyQueryMatchesEverything(t *testing.T) {
	m := Fuzzy("", "internal/tui/model.go")
	if !m.Matched {
		t.Fatal("空查询应匹配所有候选")
	}
	if len(m.Positions) != 0 {
		t.Fatalf("空查询不应有命中位置，得到 %v", m.Positions)
	}
}

func TestFuzzyGreedySubsequence(t *testing.T) {
	m := Fuzzy("mg", "main.go")
	if !m.Matched {
		t.Fatal("mg 应是 main.go 的子序列")
	}
	want := []int{0, 5}
	if !reflect.DeepEqual(m.Positions, want) {
		t.Fatalf("命中位置 = %v, 期望 %v", m.Positions, want)
	}
}

func TestFuzzyRejectsWrongOrder(t *testing.T) {
	if Fuzzy("gm", "main.go").Matched {
		t.Fatal("gm 不是 main.go 的有序子序列，应不匹配")
	}
}

func TestFuzzyRejectsMissingChars(t *testing.T) {
	if Fuzzy("mgx", "main.go").Matched {
		t.Fatal("mgx 含候选中不存在的字符，应不匹配")
	}
}

func TestFuzzyCaseInsensitive(t *testing.T) {
	if !Fuzzy("MG", "main.go").Matched {
		t.Fatal("匹配应忽略大小写")
	}
	if !Fuzzy("İN", "main.go").Matched {
		// "İN" 的全小写是 "i̇n"，包含 i 和 n，应为 main 的子序列
		t.Fatal("Unicode 小写化后的子序列应可命中")
	}
}

func TestFuzzyAndTerms(t *testing.T) {
	if !Fuzzy("mn go", "main.go").Matched {
		t.Fatal("空格分词后 mn 与 go 都命中，应匹配")
	}
	if Fuzzy("mn zz", "main.go").Matched {
		t.Fatal("任一分词未命中则整体不匹配")
	}
}

func TestFuzzyScoringPrefersBoundaryAndCompact(t *testing.T) {
	// 词首/分隔符后命中 + 更紧凑的候选应得分更高
	if Fuzzy("mg", "main.go").Score <= Fuzzy("mg", "my-great-model.go").Score {
		t.Fatalf("main.go 应排在 my-great-model.go 之前: %d vs %d",
			Fuzzy("mg", "main.go").Score, Fuzzy("mg", "my-great-model.go").Score)
	}
	// 连续命中优于分散命中
	if Fuzzy("go", "go.go").Score <= Fuzzy("go", "g-o.go").Score {
		t.Fatalf("连续命中应得分更高: %d vs %d",
			Fuzzy("go", "go.go").Score, Fuzzy("go", "g-o.go").Score)
	}
}

func TestFuzzyCamelBoundaryBonus(t *testing.T) {
	// 驼峰边界（m 后的大写 G）命中优于普通位置命中
	if Fuzzy("mg", "myGreatFile.go").Score <= Fuzzy("mg", "maXing.go").Score {
		t.Fatalf("驼峰边界命中应得分更高: %d vs %d",
			Fuzzy("mg", "myGreatFile.go").Score, Fuzzy("mg", "maXing.go").Score)
	}
}

func TestFuzzyCJKRunes(t *testing.T) {
	m := Fuzzy("模型", "用户模型.go")
	if !m.Matched {
		t.Fatal("CJK 字符按 rune 匹配")
	}
	want := []int{2, 3}
	if !reflect.DeepEqual(m.Positions, want) {
		t.Fatalf("命中位置 = %v, 期望 %v（rune 下标）", m.Positions, want)
	}
}

func TestFuzzyPositionsSortedAcrossTerms(t *testing.T) {
	m := Fuzzy("go ain", "main.go")
	if !m.Matched {
		t.Fatal("分词 go 与 ain 均命中")
	}
	for i := 1; i < len(m.Positions); i++ {
		if m.Positions[i] <= m.Positions[i-1] {
			t.Fatalf("跨词命中位置应升序去重，得到 %v", m.Positions)
		}
	}
}

// 截图案例：目录名字符拼凑出的散落匹配——分层评分后必须远低于真命中，
// 且被标记为 Scattered 供调用方过滤。
func TestFuzzySubstringHitOutranksScattered(t *testing.T) {
	real := Fuzzy("clear", "scripts/clear.sh")
	junk := Fuzzy("clear", "alibabacloud/hbrclient/c/job-0000418crpa026lfsifr_0.csv")
	if !real.Matched || !junk.Matched {
		t.Fatalf("两者都是子序列，均应命中: real=%v junk=%v", real.Matched, junk.Matched)
	}
	if !junk.Scattered {
		t.Fatal("散落拼凑的匹配应标记 Scattered")
	}
	if real.Scattered {
		t.Fatal("完整子串命中不应标记 Scattered")
	}
	if real.Score <= junk.Score {
		t.Fatalf("子串命中 (%d) 应远高于散落匹配 (%d)", real.Score, junk.Score)
	}
}

func TestFuzzyScatteredSparesAcronymAndCompact(t *testing.T) {
	// 全部命中在边界（缩写式）不算散落拼凑，即使跨度大
	m := Fuzzy("mgl", "my-great-list.go")
	if !m.Matched || m.Scattered {
		t.Fatalf("边界缩写匹配不应标记 Scattered: %+v", m)
	}
	// 紧凑的普通子序列不算散落拼凑
	m = Fuzzy("grt", "my-great-model.go")
	if !m.Matched || m.Scattered {
		t.Fatalf("紧凑命中不应标记 Scattered: %+v", m)
	}
}

func TestExactMatchRequiresSubstring(t *testing.T) {
	if ExactMatch("clear", "alibabacloud/hbrclient/c/job-0000418crpa026lfsifr_0.csv").Matched {
		t.Fatal("精确模式要求完整子串，散落子序列不应命中")
	}
	m := ExactMatch("CLEAR", "scripts/clear.sh")
	if !m.Matched {
		t.Fatal("子串命中应忽略大小写")
	}
	if ExactMatch("clear zz", "scripts/clear.sh").Matched {
		t.Fatal("分词 AND：任一词非子串则不命中")
	}
}

func TestExactMatchHighlightsAllOccurrences(t *testing.T) {
	m := ExactMatch("go", "go.go")
	if !m.Matched {
		t.Fatal("go 应命中 go.go")
	}
	want := []int{0, 1, 3, 4} // 两处出现的全部字符
	if !reflect.DeepEqual(m.Positions, want) {
		t.Fatalf("精确模式应高亮全部出现位置: %v, 期望 %v", m.Positions, want)
	}
	// 文件名内的命中应比目录名内命中的候选加分
	if ExactMatch("go", "go/a.txt").Score >= ExactMatch("go", "dir/a.go").Score {
		t.Fatal("命中在文件名内应排在目录名命中之前")
	}
}
