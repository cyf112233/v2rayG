package app

import (
	_ "embed"
	"fyne.io/fyne/v2"
)

//go:embed assets/icon.png
var iconData []byte

// appIcon 是应用图标资源(窗口与启动器共用)。
var appIcon = fyne.NewStaticResource("v2ray-gui.png", iconData)
