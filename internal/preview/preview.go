// Package preview 负责把文件渲染成可进入 viewport 的 ANSI 内容：
// 代码走 chroma 高亮，图片走 kitty/sixel 图形协议。
package preview

import (
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strings"

	"github.com/mattn/go-runewidth"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

func init() {
	// 中文环境终端常把 East Asian Ambiguous 字符（… · × 等用户文件内容里
	// 无法避免）按 2 格渲染，而宽度库默认按 1 格计。折行/截断统一按 2 格
	// 保守计算：普通终端行提前折（无害），歧义宽终端不再因行超宽触发终端
	// 软换行、导致 bubbletea 渲染器行号错位（界面鬼影）。
	// 注意：宽度走 combinedLut 查找表（包 init 时按默认配置编译），
	// 改完条件必须重建，否则不生效。
	runewidth.DefaultCondition.EastAsianWidth = true
	runewidth.DefaultCondition.CreateLUT()
}

// Kind 预览内容类型。
type Kind string

const (
	KindCode     Kind = "code"
	KindImage    Kind = "image"
	KindBinary   Kind = "binary"
	KindEmpty    Kind = "empty"
	KindTooLarge Kind = "too-large"
	KindMissing  Kind = "missing"
)

// Rendered 一份可直接进入 viewport 的渲染结果。
type Rendered struct {
	Kind     Kind
	Content  string // 完整可显示内容（含 ANSI）
	JumpLine int    // 1 起始的跳转行（真实行号）；0 表示不跳
	// JumpOffset 是 jump 行在 Content 中的物理行号（1 起始）。
	// 窗口化渲染时真实行号≠物理行号，滚动定位必须用它；0 表示退回 JumpLine。
	JumpOffset int
	Lang       string
}

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".bmp": true, ".tif": true, ".tiff": true,
}

// Render 依据文件类型与终端能力渲染预览。cols/rows 为预览区可用单元格尺寸；
// query 非空时（内容模式）在正文中高亮命中词。
func Render(path string, cols, rows int, proto Protocol, jump int, query string) (Rendered, error) {
	if imageExts[strings.ToLower(filepath.Ext(path))] {
		return renderImage(path, cols, rows, proto)
	}
	return renderCode(path, cols, jump, query)
}
