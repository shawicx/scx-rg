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
