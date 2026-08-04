package core

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// TunName 是 TUN 网卡名称,与 v4 配置 services.tun.name 保持一致。
// v2ray 核心只在进程内创建设备(需 root / CAP_NET_ADMIN,打开 /dev/net/tun),
// 系统路由需外部 ip 命令配置。
const TunName = "tun0"

// NeedsRoot 报告当前进程是否有 root 权限(TUN 设备创建与路由配置都需要)。
func NeedsRoot() bool {
	return os.Geteuid() != 0
}

// TunUp 配置系统侧路由:启用网卡、添加子网地址、添加默认路由。
// 默认路由已存在时忽略错误(幂等);ip 命令输出合并到错误信息。
func TunUp(subnet string) error {
	if err := runIP("link", "set", TunName, "up"); err != nil {
		return err
	}
	if err := runIP("addr", "add", subnet, "dev", TunName); err != nil {
		return err
	}
	// 默认路由已存在(重复连接)时报错,忽略即可。
	_ = runIP("route", "add", "default", "dev", TunName)
	return nil
}

// TunDown 还原系统路由:删除默认路由与子网地址,全部忽略错误(尽力而为)。
func TunDown(subnet string) {
	_ = runIP("route", "del", "default", "dev", TunName)
	_ = runIP("addr", "del", subnet, "dev", TunName)
}

// runIP 运行外部 ip 命令,失败时返回合并了 stderr 的错误信息。
func runIP(args ...string) error {
	cmd := exec.Command("ip", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
