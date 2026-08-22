package preview

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestRenderGifFirstFrame GIF 预览显示首帧（动画播放不在范围）：构造红→蓝
// 两帧，把 kitty payload 还原成 PNG 验证像素来自首帧。
func TestRenderGifFirstFrame(t *testing.T) {
	pal := color.Palette{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}}
	frame := func(idx uint8) *image.Paletted {
		p := image.NewPaletted(image.Rect(0, 0, 4, 4), pal)
		for i := range p.Pix {
			p.Pix[i] = idx
		}
		return p
	}
	var buf bytes.Buffer
	err := gif.EncodeAll(&buf, &gif.GIF{
		Image:  []*image.Paletted{frame(0), frame(1)},
		Delay:  []int{10, 10},
		Config: image.Config{ColorModel: pal, Width: 4, Height: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := writeTempFile(t, "anim.gif", buf.Bytes())

	ren, err := Render(path, 40, 20, ProtocolKitty, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if ren.Kind != KindImage {
		t.Fatalf("Kind = %q, 期望 image", ren.Kind)
	}
	if ren.Lang != "gif" {
		t.Errorf("Lang = %q, 期望 gif", ren.Lang)
	}

	var payload strings.Builder
	for _, chunk := range strings.Split(ren.Content, "\x1b_G") {
		semi := strings.Index(chunk, ";")
		end := strings.Index(chunk, "\x1b\\")
		if !strings.HasPrefix(chunk, "f=100") || semi < 0 || end < 0 {
			continue
		}
		payload.WriteString(chunk[semi+1 : end])
	}
	raw, err := base64.StdEncoding.DecodeString(payload.String())
	if err != nil {
		t.Fatalf("kitty payload 不是合法 base64: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("kitty payload 不是合法 PNG: %v", err)
	}
	r, g, b, _ := img.At(0, 0).RGBA()
	if r <= g || r <= b {
		t.Errorf("首帧应为红色（r>g 且 r>b），实际 rgba=%d,%d,%d", r, g, b)
	}
}

func TestRenderImageMissingFile(t *testing.T) {
	ren, err := Render(filepath.Join(t.TempDir(), "nope.png"), 40, 20, ProtocolKitty, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if ren.Kind != KindMissing {
		t.Errorf("Kind = %q, 期望 missing", ren.Kind)
	}
}

func TestRenderImageNonePlaceholder(t *testing.T) {
	path := writeTempFile(t, "img.png", pngBytes(t, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	ren, err := Render(path, 40, 20, ProtocolNone, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if ren.Kind != KindImage {
		t.Fatalf("Kind = %q, 期望 image", ren.Kind)
	}
	if strings.Contains(ren.Content, "\x1b_G") || strings.Contains(ren.Content, "\x1bP") {
		t.Errorf("none 不应输出图形序列")
	}
	if !strings.Contains(ren.Content, "禁用") {
		t.Errorf("none 占位应说明已禁用:\n%s", ren.Content)
	}
}

// TestKittyBlockDeletesOldImage kitty 输出前缀必须携带删除旧图序列：
// overlay 图形不随文本替换消失，换图/重放前不删会残留。
func TestKittyBlockDeletesOldImage(t *testing.T) {
	path := writeTempFile(t, "img.png", pngBytes(t, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	ren, err := Render(path, 40, 20, ProtocolKitty, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ren.Content, KittyDeleteImage) {
		t.Errorf("kitty 输出应带删除旧图前缀:\n%q", ren.Content)
	}
}
