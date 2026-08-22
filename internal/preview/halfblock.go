package preview

import (
	"fmt"
	"image"
	"strings"

	"github.com/muesli/termenv"
	xdraw "golang.org/x/image/draw"
)

// colorProfile 决定 halfblock 的颜色档位（TrueColor → ANSI256 → ANSI16）；
// 测试可注入，在无 TTY 环境强制出色彩（与 code.go 的 formatterFor 同一手法）。
var colorProfile = termenv.ColorProfile

// halfblockBlock 把图片渲染成「▀ 半块字符 + 前景/背景双色」的纯 ANSI 文本：
// 每个字符格承载上下两个像素行（前景=上像素、背景=下像素），不依赖任何
// 图形协议，任何支持色彩的终端都可显示——作为 kitty / sixel 均不可用时的
// 第三档渲染。图片等比缩放进 cols×rows 格子框（小图放大，铺满可用区域）。
func halfblockBlock(img image.Image, cols, rows int) (string, error) {
	p := colorProfile()
	b := img.Bounds()
	if p == termenv.Ascii || b.Dx() <= 0 || b.Dy() <= 0 || cols <= 0 || rows <= 0 {
		return "", nil
	}
	// 目标像素分辨率：cols 列 × 2·rows 行（每字符格纵向承载 2 像素行）
	s := min(float64(cols)/float64(b.Dx()), float64(2*rows)/float64(b.Dy()))
	c := max(1, int(float64(b.Dx())*s))
	ph := max(1, int(float64(b.Dy())*s))
	dst := image.NewRGBA(image.Rect(0, 0, c, ph))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)

	// 同色像素（纯色图、截图大片底色）只量化一次；相邻同色跳过重设 SGR
	seqs := make(map[[3]byte][2]string, 256)
	sgr := func(col [3]byte, bg bool) string {
		v, ok := seqs[col]
		if !ok {
			conv := p.Convert(termenv.RGBColor(fmt.Sprintf("#%02x%02x%02x", col[0], col[1], col[2])))
			v = [2]string{conv.Sequence(false), conv.Sequence(true)}
			seqs[col] = v
		}
		if bg {
			return v[1]
		}
		return v[0]
	}

	var sb strings.Builder
	black := [3]byte{}
	for y := 0; y < ph; y += 2 {
		var lastFg, lastBg [3]byte
		haveFg, haveBg := false, false
		for x := 0; x < c; x++ {
			top := pixelAt(dst, x, y)
			bot := black
			if y+1 < ph {
				bot = pixelAt(dst, x, y+1)
			}
			if !haveFg || top != lastFg {
				fmt.Fprintf(&sb, "\x1b[%sm", sgr(top, false))
				lastFg, haveFg = top, true
			}
			if !haveBg || bot != lastBg {
				fmt.Fprintf(&sb, "\x1b[%sm", sgr(bot, true))
				lastBg, haveBg = bot, true
			}
			sb.WriteString("▀")
		}
		sb.WriteString("\x1b[0m")
		if y+2 < ph {
			sb.WriteString("\n")
		}
	}
	return sb.String(), nil
}

// pixelAt 取缩放后的像素颜色。image.RGBA 是预乘 alpha 存储，透明区
// （Over 到全零底）本就是黑；此处再把半透明边缘反预乘回原色，
// 低不透明度统一按黑处理，避免出现意外亮斑。
func pixelAt(img *image.RGBA, x, y int) [3]byte {
	c := img.RGBAAt(x, y)
	if c.A < 128 {
		return [3]byte{}
	}
	if c.A == 255 {
		return [3]byte{c.R, c.G, c.B}
	}
	a := uint32(c.A)
	return [3]byte{
		uint8(uint32(c.R) * 255 / a),
		uint8(uint32(c.G) * 255 / a),
		uint8(uint32(c.B) * 255 / a),
	}
}
