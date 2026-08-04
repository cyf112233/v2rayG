//go:build linux

package helper

import (
	"bytes"
	"math/rand"
	"os"
	"testing"

	v2raycore "github.com/v2fly/v2ray-core/v5"
	_ "github.com/v2fly/v2ray-core/v5/main/distro/all" // 注册 json 格式与全部 proxy/transport

	"v2ray-gui/internal/core"
)

// liveTestServer 构造用于链路测试的服务器(上游指向未监听端口,不发起真实代理)。
func liveTestServer() *core.Server {
	return &core.Server{
		ID:        core.NewID(),
		Name:      "链路测试",
		Protocol:  "vmess",
		Address:   "127.0.0.1",
		Port:      1,
		UUID:      "00000000-0000-0000-0000-000000000000",
		AlterID:   0,
		Security:  "auto",
		Network:   "tcp",
		TLS:       "none",
		LatencyMS: -1,
	}
}

// TestLivePreopenedFDChain 验证整条链路(需外部已启动常驻 root 助手):
// 非 root 进程经 SCM_RIGHTS 收到 root 创建的 tun fd → 配置 preopened_fd →
// 内嵌核心直接使用该 fd 启动、停止。设置环境变量 V2RAYG_LIVE_TEST=1 且
// socket 存在时才会执行,否则跳过(普通 go test/CI 不受影响)。
func TestLivePreopenedFDChain(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if os.Getenv("V2RAYG_LIVE_TEST") != "1" {
		t.Skip("需要 V2RAYG_LIVE_TEST=1 且常驻 root 助手在运行")
	}
	client, err := EnsureClient()
	if err != nil {
		t.Fatalf("EnsureClient: %v", err)
	}
	f, err := client.OpenTun(core.TunName)
	if err != nil {
		t.Fatalf("OpenTun: %v", err)
	}
	defer f.Close()

	st := core.Settings{
		SocksPort: 20000 + rand.Intn(10000),
		HTTPPort:  20000 + rand.Intn(10000),
		TunEnable: true,
		TunSubnet: "10.99.0.1/24",
		TunMTU:    1500,
		TunFD:     int(f.Fd()),
		RouteMode: "direct",
		LogLevel:  "error",
	}
	cfgBytes, err := core.BuildV2rayConfig(liveTestServer(), st)
	if err != nil {
		t.Fatalf("BuildV2rayConfig: %v", err)
	}
	cfg, err := v2raycore.LoadConfig("json", bytes.NewReader(cfgBytes))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	inst, err := v2raycore.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := inst.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := inst.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	t.Log("preopened_fd 链路:OpenTun → 核心 Start/Stop 成功(非 root 使用 root 创建的 tun fd)")
}
