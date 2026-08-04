package app

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

// TestAppIconEmbedded 校验内嵌的应用图标是有效的 512x512 PNG,确保
// go:embed 的资源能在运行时被 Fyne 正常解码。
func TestAppIconEmbedded(t *testing.T) {
	if len(iconData) == 0 {
		t.Fatal("iconData is empty, go:embed failed")
	}
	img, err := png.Decode(bytes.NewReader(iconData))
	if err != nil {
		t.Fatalf("embedded icon is not a valid PNG: %v", err)
	}
	if got := img.Bounds().Dx(); got != 512 {
		t.Errorf("icon width = %d, want 512", got)
	}
	if got := img.Bounds().Dy(); got != 512 {
		t.Errorf("icon height = %d, want 512", got)
	}
	if appIcon == nil || appIcon.Name() != "v2ray-gui.png" {
		t.Errorf("appIcon not initialized correctly: %+v", appIcon)
	}
	// 圆角图标四个角应透明(alpha=0)。
	if _, ok := img.(*image.NRGBA); ok {
		n := img.(*image.NRGBA)
		for _, c := range []image.Point{{0, 0}, {511, 0}, {0, 511}, {511, 511}} {
			if a := n.NRGBAAt(c.X, c.Y).A; a != 0 {
				t.Errorf("corner %v alpha = %d, want 0 (rounded corners)", c, a)
			}
		}
	}
}
