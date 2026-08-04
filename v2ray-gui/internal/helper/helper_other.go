//go:build !linux

package helper

import (
	"fmt"
	"os"
)

// Run 在非 Linux 平台为桩实现:常驻 root 助手仅 Linux 支持。
func Run() {
	fmt.Fprintln(os.Stderr, "v2rayg-helper: 仅支持 Linux")
	os.Exit(1)
}
