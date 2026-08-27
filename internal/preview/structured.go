package preview

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
)

// 结构化预览：JSON 缩进树与 CSV/TSV 对齐表格。
// 降级规则（设计约束）：结构化视图是「格式化重排」，行号不对应原始文件，
// 因此禁用 jump-to-line / 命中行跳转；查询高亮与日志级别着色照常生效
// （renderLines 管线不变）。YAML 走既有 chroma 语法高亮（无树化）。

const (
	structuredMaxRows = 500 // CSV 渲染行上限
	structuredMaxCols = 30  // CSV 渲染列上限
	structuredColW    = 40  // 单列显示宽度上限
)

// structuredKind 按扩展名识别结构化类型；空串 = 非结构化。
func structuredKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "JSON"
	case ".csv":
		return "CSV"
	case ".tsv":
		return "TSV"
	}
	return ""
}

// renderJSONLines 把原始 JSON 缩进为树形（json.Indent 不重解析，
// 数字精度与键序原样保留）；非法 JSON 返回 ok=false（回退普通代码渲染）。
func renderJSONLines(raw []byte) ([]codeLine, bool) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, bytes.TrimSpace(raw), "", "  "); err != nil {
		return nil, false
	}
	out := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(out) > maxCodeLines {
		out = out[:maxCodeLines]
	}
	lines := make([]codeLine, len(out))
	for i, t := range out {
		lines[i] = codeLine{no: i + 1, text: t}
	}
	return lines, true
}

// csvLines 把 CSV/TSV 渲染为对齐表格文本：列宽 = 该列最大显示宽
// （封顶 structuredColW），列间两空格；行数超限截断并加省略行。
func csvLines(raw []byte, comma rune) []codeLine {
	r := csv.NewReader(strings.NewReader(string(raw)))
	r.Comma = comma
	r.FieldsPerRecord = -1 // 行列数不强制一致
	records, err := r.ReadAll()
	if err != nil || len(records) == 0 {
		return nil
	}
	total := len(records)
	if total > structuredMaxRows {
		records = records[:structuredMaxRows]
	}
	nCols := 0
	for _, rec := range records {
		nCols = max(nCols, len(rec))
	}
	nCols = min(nCols, structuredMaxCols)
	widths := make([]int, nCols)
	for _, rec := range records {
		for c := 0; c < nCols && c < len(rec); c++ {
			widths[c] = min(structuredColW, max(widths[c], lipgloss.Width(rec[c])))
		}
	}
	lines := make([]codeLine, 0, len(records)+1)
	for ri, rec := range records {
		var b strings.Builder
		for c := 0; c < nCols; c++ {
			if c > 0 {
				b.WriteString("  ")
			}
			cell := ""
			if c < len(rec) {
				cell = rec[c]
			}
			if pad := widths[c] - lipgloss.Width(cell); pad > 0 {
				b.WriteString(cell)
				b.WriteString(strings.Repeat(" ", pad))
			} else {
				b.WriteString(cell)
			}
		}
		lines = append(lines, codeLine{no: ri + 1, text: strings.TrimRight(b.String(), " ")})
	}
	if total > structuredMaxRows {
		lines = append(lines, codeLine{no: 0,
			text: fmt.Sprintf("⋯ 共 %d 行，仅显示前 %d 行", total, structuredMaxRows)})
	}
	return lines
}
