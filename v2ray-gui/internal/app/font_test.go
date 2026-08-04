package app

import (
	"testing"

	"golang.org/x/image/font/opentype"
)

// TestNotoFontEmbedded 验证内嵌的 Noto Sans SC 字体可被 opentype 解析。
func TestNotoFontEmbedded(t *testing.T) {
	if len(fontData) == 0 {
		t.Fatal("字体未通过 go:embed 嵌入(fontData 为空)")
	}
	if _, err := opentype.Parse(fontData); err != nil {
		t.Fatalf("opentype.Parse 失败:%v", err)
	}
	if got := notoFont.Name(); got != "NotoSansSC-Regular.otf" {
		t.Fatalf("字体资源名不正确:got %q, want %q", got, "NotoSansSC-Regular.otf")
	}
}
