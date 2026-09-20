package tui

// 诊断探针（临时）：图片预览花屏问题定位——验证 kitty 图形流经过
// preview.Render → applyPreview → frame() 后是否字节级完整。
// 定位结论得出后本文件或删除或改写为回归测试。

import (
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"scx-rg/internal/preview"
)

func TestProbeKittyFrameIntegrity(t *testing.T) {
	// 生成 4126x2330 测试图（彩色色块，接近用户截图中的图表尺寸）
	img := image.NewRGBA(image.Rect(0, 0, 4126, 2330))
	for x := 0; x < 4126; x += 300 {
		for y := 0; y < 2330; y += 300 {
			c := color.NRGBA{uint8(x / 300 % 256), uint8(y / 300 % 256), uint8((x + y) / 600 % 256), 255}
			draw.Draw(img, image.Rect(x, y, min(x+200, 4126), min(y+200, 2330)), &image.Uniform{c}, image.Point{}, draw.Src)
		}
	}
	path := filepath.Join(t.TempDir(), "probe.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	m := New(Config{Mode: ModeContent})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	ren, err := preview.Render(path, 60, 30, preview.ProtocolKitty, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	m.prevPath = path
	m.applyPreview(path, ren, nil)
	frame := m.frame()

	// 1) APC 起止数量守恒
	starts := strings.Count(frame, "\x1b_G")
	ends := strings.Count(frame, "\x1b\\")
	t.Logf("APC starts=%d ends=%d frameBytes=%d", starts, ends, len(frame))
	if starts != ends {
		t.Fatalf("APC 起止不配对: starts=%d ends=%d", starts, ends)
	}
	if starts == 0 {
		t.Fatal("帧内未发现任何 kitty 图形序列")
	}

	// 2) 逐块提取载荷，块内不得混入换行/其他转义（正则严格限定 base64 字符集）
	chunkRe := regexp.MustCompile(`\x1b_Gf=100,a=T,q=1,i=7,c=\d+,r=\d+,m=(\d);([A-Za-z0-9+/=]*)\x1b\\`)
	matches := chunkRe.FindAllStringSubmatch(frame, -1)
	if len(matches) != starts-1 { // 减去删除序列 a=d
		t.Fatalf("正则匹配到 %d 个数据块, APC 总数 %d（含 1 个删除序列）——存在被污染的序列", len(matches), starts)
	}
	var payload strings.Builder
	for i, mt := range matches {
		if i == 0 {
			re := regexp.MustCompile(`\x1b_Gf=100,a=T,q=1,i=7,c=(\d+),r=(\d+),m=\d;`)
			if pm := re.FindStringSubmatch(frame); pm != nil {
				t.Logf("placement: c=%s r=%s", pm[1], pm[2])
			}
		}
		payload.WriteString(mt[2])
	}
	// 3) base64 解码并重新按 PNG 解码
	raw, err := base64.StdEncoding.DecodeString(payload.String())
	if err != nil {
		t.Fatalf("base64 解码失败（载荷被污染）: %v", err)
	}
	decoded, err := png.Decode(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("PNG 解码失败（帧内载荷损坏）: %v", err)
	}
	t.Logf("decoded: %dx%d, src: %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy(), img.Bounds().Dx(), img.Bounds().Dy())
	if decoded.Bounds().Dx() != img.Bounds().Dx() || decoded.Bounds().Dy() != img.Bounds().Dy() {
		t.Fatalf("解码尺寸不符: got %v want %v", decoded.Bounds(), img.Bounds())
	}
	// 4) 像素抽查：首色块颜色一致
	if got := decoded.At(0, 0); got != (color.NRGBA{0, 0, 0, 255}) {
		t.Fatalf("像素不符: got %v", got)
	}
	t.Log("PASS: 帧内 kitty 流字节级完整")
}
