package main

import (
	"os"

	"v2ray-gui/internal/app"
	"v2ray-gui/internal/helper"
)

func main() {
	// 常驻 root 助手:由主程序 pkexec env V2RAYG_HELPER=1 ... <self> 启动,
	// 无 GUI,以 root 处理 TUN 设备创建与路由配置(见 internal/helper)。
	if os.Getenv("V2RAYG_HELPER") == "1" {
		helper.Run()
		return
	}
	app.Run()
}
