package engine

import (
	"fmt"
	"math/rand"
	"net"
	"testing"
	"time"

	"v2ray-gui/internal/core"
)

// randomPort 生成 20000-39999 的随机端口,避免与用户运行中的服务冲突。
func randomPort() int {
	return 20000 + rand.Intn(20000)
}

// newTestServer 构造用于集成测试的服务器。
// 上游指向 127.0.0.1:1(未使用的端口),测试只做 SOCKS5 问候握手,
// 不发起真正的代理连接,因此不需要外网,等价于 freedom 出站场景。
func newTestServer() *core.Server {
	return &core.Server{
		ID:        core.NewID(),
		Name:      "测试节点",
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

// TestEngineStartStop 集成测试:真正启动内嵌核心,验证 SOCKS 端口可用、
// 流量统计不 panic、Stop 后状态正确。
func TestEngineStartStop(t *testing.T) {
	socksPort := randomPort()
	httpPort := randomPort()
	for httpPort == socksPort {
		httpPort = randomPort()
	}
	st := core.Settings{SocksPort: socksPort, HTTPPort: httpPort}

	var lines []string
	e := NewEngine(func(line string) {
		lines = append(lines, line)
	})
	// 无论测试如何结束都确保 Stop。
	t.Cleanup(e.Stop)

	if err := e.Start(newTestServer(), st); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !e.Running() {
		t.Fatal("Start 后 Running() 应为 true")
	}
	if e.Uptime() <= 0 {
		t.Error("Start 后 Uptime() 应为正")
	}

	// 2 秒内重试连接 SOCKS 端口(核心启动有异步时序)。
	addr := fmt.Sprintf("127.0.0.1:%d", socksPort)
	deadline := time.Now().Add(2 * time.Second)
	var conn net.Conn
	var dialErr error
	for time.Now().Before(deadline) {
		conn, dialErr = net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if dialErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if dialErr != nil {
		t.Fatalf("SOCKS 端口未就绪: %v", dialErr)
	}
	defer conn.Close()

	// SOCKS5 问候:版本 5、1 种认证方法、无认证。
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatalf("发送问候失败: %v", err)
	}
	reply := make([]byte, 2)
	if _, err := conn.Read(reply); err != nil {
		t.Fatalf("读取问候响应失败: %v", err)
	}
	if reply[0] != 0x05 || reply[1] != 0x00 {
		t.Fatalf("意外响应: %v", reply)
	}

	// Traffic 查询不应 panic。
	up, down := e.Traffic()
	t.Logf("traffic up=%d down=%d", up, down)

	e.Stop()
	if e.Running() {
		t.Fatal("Stop 后 Running() 应为 false")
	}
	// Stop 幂等。
	e.Stop()
	// Stop 后 Uptime 为 0、Traffic 为 0。
	if e.Uptime() != 0 {
		t.Errorf("Stop 后 Uptime=%v, want 0", e.Uptime())
	}
	if up2, down2 := e.Traffic(); up2 != 0 || down2 != 0 {
		t.Errorf("Stop 后 Traffic=(%d,%d), want (0,0)", up2, down2)
	}
	if len(lines) == 0 {
		t.Log("核心未输出日志(可能被 loglevel 过滤)")
	}
}

// TestEngineStartInvalidProtocol 验证未知协议返回错误。
func TestEngineStartInvalidProtocol(t *testing.T) {
	e := NewEngine(nil)
	defer e.Stop()
	s := newTestServer()
	s.Protocol = "unknown"
	if err := e.Start(s, core.Settings{SocksPort: randomPort(), HTTPPort: randomPort()}); err == nil {
		t.Fatal("未知协议应返回错误")
	}
}
