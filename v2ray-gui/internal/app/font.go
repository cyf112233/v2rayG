package app

import (
	_ "embed" // 供 go:embed 使用([]byte 变量需要 blank import)
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

//go:embed assets/fonts/NotoSansSC-Regular.otf
var fontData []byte

var notoFont = fyne.NewStaticResource("NotoSansSC-Regular.otf", fontData)

// notoTheme 以内置的 Noto Sans SC 作为界面字体,避免依赖系统字体。
// 等宽字体保留默认(日志 TextGrid 需要等宽对齐)。
type notoTheme struct{}

func (notoTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return theme.DefaultTheme().Color(n, v)
}

func (notoTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

// Font 返回界面字体:等宽场景用默认字体,保证日志 TextGrid 对齐。
func (notoTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Monospace {
		return theme.DefaultTheme().Font(style)
	}
	return notoFont
}

func (notoTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNameText {
		return 15 // 略大字号,界面更舒适
	}
	return theme.DefaultTheme().Size(n)
}
