package app

import (
	"os"
	"os/exec"
	"path/filepath"
)

// relaunchElevated 通过 pkexec 以 root 重启自身。
// 必须显式传递 XDG_CONFIG_HOME(否则 root 的 HOME 不同,配置会读写到别处),
// 并通过 V2RAY_GUI_CONNECT 让重启后的实例自动连接指定服务器。
func relaunchElevated(connectID string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	// 计算用户实际的配置目录:优先 XDG_CONFIG_HOME,否则 ~/.config
	cfgHome := os.Getenv("XDG_CONFIG_HOME")
	if cfgHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		cfgHome = filepath.Join(home, ".config")
	}
	args := []string{"env",
		"XDG_CONFIG_HOME=" + cfgHome,
		"V2RAY_GUI_ELEVATED=1",
		"V2RAY_GUI_CONNECT=" + connectID,
		self,
	}
	cmd := exec.Command("pkexec", args...)
	cmd.Stdout, cmd.Stderr = nil, nil // 交给 pkexec 自己的界面/终端
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil // 不等待,立即退出父进程
}
