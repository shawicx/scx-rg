package preview

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-sixel"
	xdraw "golang.org/x/image/draw"
)

const kittyChunkSize = 4096

var styleImgBox = lipgloss.NewStyle().
	Border(lipgloss.Border{ // ASCII 边框：Unicode 制表符是歧义宽字符，见 tui/styles.go 说明
		Top: "-", Bottom: "-", Left: "|", Right: "|",
		TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
	}).
	BorderForeground(lipgloss.Color("#626262")).
	Padding(1, 2).
	Foreground(lipgloss.Color("#626262"))

func renderImage(path string, cols, rows int, proto Protocol) (Rendered, error) {
	f, err := os.Open(path)
	if err != nil {
		return Rendered{Kind: KindMissing, Content: "无法读取图片: " + err.Error()}, nil
	}
	defer f.Close()
	img, format, err := image.Decode(f)
	if err != nil {
		return Rendered{Kind: KindMissing, Content: "图片解码失败: " + err.Error()}, nil
	}
	info := fmt.Sprintf("%s / %s / %dx%dpx",
		filepath.Base(path), format, img.Bounds().Dx(), img.Bounds().Dy())

	if proto == ProtocolNone || proto == ProtocolAuto || cols <= 0 || rows <= 0 {
		return Rendered{Kind: KindImage, Content: placeholderImage(info, proto), Lang: format}, nil
	}

	cw, ch := cellSize()
	var block string
	switch proto {
	case ProtocolKitty:
		block, err = kittyBlock(img, cols, rows, cw, ch)
	case ProtocolSixel:
		block, err = sixelBlock(img, cols, rows, cw, ch)
	}
	if err != nil {
		return Rendered{Kind: KindImage, Content: "图片渲染失败: " + err.Error()}, nil
	}
	return Rendered{Kind: KindImage, Content: info + "\n" + block, Lang: format}, nil
}

func placeholderImage(info string, proto Protocol) string {
	lines := []string{
		"🖼  " + info,
		"",
		"当前终端未检测到 kitty / sixel 图形协议（" + string(proto) + "）",
		"推荐在 kitty / ghostty / wezterm 中运行，",
		"或通过 --img kitty / --img sixel 强制指定",
	}
	return styleImgBox.Render(strings.Join(lines, "\n"))
}

// fitCells 在 cols×rows 的单元格框内按等宽 cell 像素比计算等比缩放后的占位尺寸。
func fitCells(imgW, imgH, cols, rows, cw, ch int) (c, r int) {
	if imgW <= 0 || imgH <= 0 || cols <= 0 || rows <= 0 || cw <= 0 || ch <= 0 {
		return 0, 0
	}
	bw, bh := cols*cw, rows*ch
	s := 1.0
	if imgW > bw || imgH > bh {
		s = min(float64(bw)/float64(imgW), float64(bh)/float64(imgH))
	}
	c = int(float64(imgW) * s / float64(cw))
	r = int(float64(imgH) * s / float64(ch))
	return min(max(c, 1), cols), min(max(r, 1), rows)
}

// kittyBlock 用 kitty 图形协议输出图片（整图 PNG base64，c/r 指定占位单元格数）。
// 固定 i=7 让重复渲染复用同一 id，避免图像在终端内堆积。
// 末尾补 r-1 个空行占位，使 viewport 行数与图像视觉行数一致。
func kittyBlock(img image.Image, cols, rows, cw, ch int) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	b := img.Bounds()
	c, r := fitCells(b.Dx(), b.Dy(), cols, rows, cw, ch)
	if c <= 0 || r <= 0 {
		return "", nil
	}
	payload := base64.StdEncoding.EncodeToString(buf.Bytes())
	var sb strings.Builder
	for off := 0; off < len(payload); off += kittyChunkSize {
		end := min(off+kittyChunkSize, len(payload))
		m := 1
		if end == len(payload) {
			m = 0
		}
		fmt.Fprintf(&sb, "\x1b_Gf=100,a=T,q=1,i=7,c=%d,r=%d,m=%d;%s\x1b\\", c, r, m, payload[off:end])
	}
	for i := 1; i < r; i++ {
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

// sixelBlock 用 sixel 协议输出图片：先缩放到面板像素框再编码。
func sixelBlock(img image.Image, cols, rows, cw, ch int) (string, error) {
	scaled := scaleFit(img, cols*cw, rows*ch)
	var buf bytes.Buffer
	enc := sixel.NewEncoder(&buf)
	if err := enc.Encode(scaled); err != nil {
		return "", err
	}
	visRows := (scaled.Bounds().Dy() + ch - 1) / ch
	var sb strings.Builder
	sb.Write(buf.Bytes())
	for i := 1; i < visRows; i++ {
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func scaleFit(src image.Image, maxW, maxH int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || maxW <= 0 || maxH <= 0 || (w <= maxW && h <= maxH) {
		return src
	}
	s := min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	nw, nh := max(1, int(float64(w)*s)), max(1, int(float64(h)*s))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst
}
