package core

import (
	"encoding/json"
	"strings"
	"testing"
)

const testUUID = "00000000-0000-0000-0000-000000000000"

// baseServer 构造一个带默认值的测试服务器。
func baseServer(protocol string) *Server {
	return &Server{
		ID:        "test-id",
		Name:      "测试",
		Protocol:  protocol,
		Address:   "example.com",
		Port:      443,
		UUID:      testUUID,
		Network:   "tcp",
		TLS:       "none",
		LatencyMS: -1,
	}
}

func testSettings() Settings {
	return Settings{SocksPort: 10808, HTTPPort: 10809}
}

// parseCfg 生成配置并解析为 map,便于断言;同时校验 json.Valid。
func parseCfg(t *testing.T, s *Server, st Settings) map[string]interface{} {
	t.Helper()
	data, err := BuildV2rayConfig(s, st)
	if err != nil {
		t.Fatalf("BuildV2rayConfig: %v", err)
	}
	if !json.Valid(data) {
		t.Fatal("生成的配置不是合法 JSON")
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	return m
}

func obj(m map[string]interface{}, key string) map[string]interface{} {
	v, ok := m[key].(map[string]interface{})
	if !ok {
		panic("缺少字段或类型错误: " + key)
	}
	return v
}

func arr(m map[string]interface{}, key string) []interface{} {
	v, ok := m[key].([]interface{})
	if !ok {
		panic("缺少字段或类型错误: " + key)
	}
	return v
}

// proxyOutbound 返回第一个出站(代理出站)。
func proxyOutbound(m map[string]interface{}) map[string]interface{} {
	return arr(m, "outbounds")[0].(map[string]interface{})
}

// firstVnextUser 返回 vmess/vless 出站 settings.vnext[0].users[0]。
func firstVnextUser(out map[string]interface{}) map[string]interface{} {
	vnext := arr(obj(out, "settings"), "vnext")
	server := vnext[0].(map[string]interface{})
	users := arr(server, "users")
	return users[0].(map[string]interface{})
}

func TestConfigStatsAndPolicy(t *testing.T) {
	m := parseCfg(t, baseServer("vmess"), testSettings())
	if _, ok := m["stats"]; !ok {
		t.Error("缺少 stats 字段")
	}
	system := obj(obj(m, "policy"), "system")
	if system["statsInboundUplink"] != true || system["statsInboundDownlink"] != true {
		t.Errorf("policy.system 缺少流量统计开关: %v", system)
	}
}

func TestConfigRoutingNoGeoIP(t *testing.T) {
	m := parseCfg(t, baseServer("vmess"), testSettings())
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "geoip") {
		t.Error("路由不应包含 geoip:private,避免依赖 geoip.dat 资产文件")
	}
	routing := obj(m, "routing")
	if routing["domainStrategy"] != "IPIfNonMatch" {
		t.Errorf("domainStrategy=%v", routing["domainStrategy"])
	}
	rules := arr(routing, "rules")
	if len(rules) == 0 {
		t.Fatal("缺少路由规则")
	}
	rule := rules[0].(map[string]interface{})
	ips := arr(rule, "ip")
	if len(ips) == 0 {
		t.Error("路由规则缺少私有网段列表")
	}
	if rule["outboundTag"] != "direct" {
		t.Errorf("outboundTag=%v", rule["outboundTag"])
	}
}

// TestBuildConfigRouteModes 校验三种路由模式(及空值兼容)生成的路由规则。
func TestBuildConfigRouteModes(t *testing.T) {
	base := baseServer("vmess")

	t.Run("rules", func(t *testing.T) {
		st := testSettings()
		st.RouteMode = "rules"
		m := parseCfg(t, base, st)
		rules := arr(obj(m, "routing"), "rules")
		if len(rules) == 0 {
			t.Fatal("rules 模式应包含私有网段直连规则")
		}
		rule := rules[0].(map[string]interface{})
		if rule["outboundTag"] != "direct" {
			t.Errorf("outboundTag=%v, want direct", rule["outboundTag"])
		}
		ips := arr(rule, "ip")
		found := false
		for _, ip := range ips {
			if ip == "10.0.0.0/8" {
				found = true
			}
		}
		if !found {
			t.Errorf("规则 ip 应包含 10.0.0.0/8,got %v", ips)
		}
	})

	t.Run("global", func(t *testing.T) {
		st := testSettings()
		st.RouteMode = "global"
		m := parseCfg(t, base, st)
		rules := arr(obj(m, "routing"), "rules")
		if len(rules) != 0 {
			t.Errorf("global 模式 rules 应为空数组,got %v", rules)
		}
	})

	t.Run("direct", func(t *testing.T) {
		st := testSettings()
		st.RouteMode = "direct"
		m := parseCfg(t, base, st)
		rules := arr(obj(m, "routing"), "rules")
		if len(rules) != 1 {
			t.Fatalf("direct 模式应只有一条兜底规则,got %d 条", len(rules))
		}
		rule := rules[0].(map[string]interface{})
		if rule["network"] != "tcp,udp" {
			t.Errorf("network=%v, want tcp,udp", rule["network"])
		}
		if rule["outboundTag"] != "direct" {
			t.Errorf("outboundTag=%v, want direct", rule["outboundTag"])
		}
	})

	t.Run("empty defaults to rules", func(t *testing.T) {
		// RouteMode 未设置(旧配置)时与 "rules" 等价。
		mEmpty := parseCfg(t, base, testSettings())
		stRules := testSettings()
		stRules.RouteMode = "rules"
		mRules := parseCfg(t, base, stRules)
		rawEmpty, err := json.Marshal(mEmpty)
		if err != nil {
			t.Fatal(err)
		}
		rawRules, err := json.Marshal(mRules)
		if err != nil {
			t.Fatal(err)
		}
		if string(rawEmpty) != string(rawRules) {
			t.Error("未设置 RouteMode 时应与 rules 等价")
		}
	})
}

func TestConfigInboundPorts(t *testing.T) {
	st := Settings{SocksPort: 20001, HTTPPort: 20002}
	m := parseCfg(t, baseServer("vmess"), st)
	inbounds := arr(m, "inbounds")
	if len(inbounds) != 2 {
		t.Fatalf("inbounds=%d, want 2", len(inbounds))
	}
	socks := inbounds[0].(map[string]interface{})
	http := inbounds[1].(map[string]interface{})
	if socks["port"] != float64(20001) || socks["protocol"] != "socks" {
		t.Errorf("socks inbound: %v", socks)
	}
	if http["port"] != float64(20002) || http["protocol"] != "http" {
		t.Errorf("http inbound: %v", http)
	}
	// 第二个出站必须是 freedom(direct)。
	direct := arr(m, "outbounds")[1].(map[string]interface{})
	if direct["protocol"] != "freedom" || direct["tag"] != "direct" {
		t.Errorf("direct outbound: %v", direct)
	}
}

func TestConfigVmess(t *testing.T) {
	s := baseServer("vmess")
	s.AlterID = 64
	// Security 留空 → 默认 "auto"。
	m := parseCfg(t, s, testSettings())
	out := proxyOutbound(m)
	if out["protocol"] != "vmess" || out["tag"] != "proxy" {
		t.Errorf("outbound: %v", out)
	}
	users := firstVnextUser(out)
	u := users
	if u["id"] != testUUID {
		t.Errorf("id=%v", u["id"])
	}
	if u["alterId"] != float64(64) {
		t.Errorf("alterId=%v", u["alterId"])
	}
	if u["security"] != "auto" {
		t.Errorf("security 默认应为 auto: %v", u["security"])
	}
	ss := obj(out, "streamSettings")
	if ss["network"] != "tcp" {
		t.Errorf("network=%v", ss["network"])
	}
	if _, ok := ss["security"]; ok {
		t.Error("TLS 为 none 时不应输出 security 字段")
	}
}

func TestConfigVmessSecurity(t *testing.T) {
	s := baseServer("vmess")
	s.Security = "aes-128-gcm"
	m := parseCfg(t, s, testSettings())
	u := firstVnextUser(proxyOutbound(m))
	if u["security"] != "aes-128-gcm" {
		t.Errorf("security=%v", u["security"])
	}
}

func TestConfigVless(t *testing.T) {
	s := baseServer("vless")
	s.Flow = "xtls-rprx-vision"
	m := parseCfg(t, s, testSettings())
	out := proxyOutbound(m)
	if out["protocol"] != "vless" {
		t.Errorf("protocol=%v", out["protocol"])
	}
	u := firstVnextUser(out)
	if u["encryption"] != "none" {
		t.Errorf("encryption=%v, want none", u["encryption"])
	}
	if u["flow"] != "xtls-rprx-vision" {
		t.Errorf("flow=%v", u["flow"])
	}
}

func TestConfigVlessNoFlow(t *testing.T) {
	s := baseServer("vless")
	m := parseCfg(t, s, testSettings())
	u := firstVnextUser(proxyOutbound(m))
	if _, ok := u["flow"]; ok {
		t.Error("flow 为空时不应输出 flow 字段")
	}
}

func TestConfigTrojan(t *testing.T) {
	s := baseServer("trojan")
	s.UUID = "pass-trojan"
	s.Flow = "xtls-rprx-vision"
	m := parseCfg(t, s, testSettings())
	out := proxyOutbound(m)
	if out["protocol"] != "trojan" {
		t.Errorf("protocol=%v", out["protocol"])
	}
	server := arr(obj(out, "settings"), "servers")[0].(map[string]interface{})
	if server["address"] != "example.com" || server["port"] != float64(443) ||
		server["password"] != "pass-trojan" || server["level"] != float64(0) {
		t.Errorf("server: %v", server)
	}
	if server["flow"] != "xtls-rprx-vision" {
		t.Errorf("flow=%v", server["flow"])
	}
}

func TestConfigSS(t *testing.T) {
	s := baseServer("shadowsocks")
	s.Method = "aes-256-gcm"
	s.Password = "ss-pass"
	m := parseCfg(t, s, testSettings())
	out := proxyOutbound(m)
	if out["protocol"] != "shadowsocks" {
		t.Errorf("protocol=%v", out["protocol"])
	}
	server := arr(obj(out, "settings"), "servers")[0].(map[string]interface{})
	if server["method"] != "aes-256-gcm" || server["password"] != "ss-pass" {
		t.Errorf("server: %v", server)
	}
}

func TestConfigRealityError(t *testing.T) {
	s := baseServer("vless")
	s.TLS = "reality"
	_, err := BuildV2rayConfig(s, testSettings())
	if err == nil {
		t.Fatal("reality 应返回错误")
	}
	if !strings.Contains(err.Error(), "Xray") {
		t.Errorf("错误信息应提示 Xray-core: %v", err)
	}
}

func TestConfigWSTLS(t *testing.T) {
	s := baseServer("vmess")
	s.Network = "ws"
	s.TLS = "tls"
	s.Host = "cdn.example.com"
	s.Path = "/v2"
	s.SNI = "sni.example.com"
	s.ALPN = []string{"h2", "http/1.1"}
	s.Fingerprint = "chrome"
	m := parseCfg(t, s, testSettings())
	ss := obj(proxyOutbound(m), "streamSettings")
	if ss["network"] != "ws" {
		t.Errorf("network=%v", ss["network"])
	}
	ws := obj(ss, "wsSettings")
	if ws["path"] != "/v2" {
		t.Errorf("ws path=%v", ws["path"])
	}
	headers := obj(ws, "headers")
	if headers["Host"] != "cdn.example.com" {
		t.Errorf("ws headers.Host=%v", headers["Host"])
	}
	if ss["security"] != "tls" {
		t.Errorf("security=%v", ss["security"])
	}
	tls := obj(ss, "tlsSettings")
	if tls["serverName"] != "sni.example.com" {
		t.Errorf("serverName 应优先取 SNI: %v", tls["serverName"])
	}
	if tls["allowInsecure"] != true {
		t.Errorf("allowInsecure=%v", tls["allowInsecure"])
	}
	if len(arr(tls, "alpn")) != 2 {
		t.Errorf("alpn=%v", tls["alpn"])
	}
	if tls["fingerprint"] != "chrome" {
		t.Errorf("fingerprint=%v", tls["fingerprint"])
	}
}

func TestConfigServerNameFallback(t *testing.T) {
	// SNI、Host 都为空 → serverName 回退到 Address。
	s := baseServer("vless")
	s.Network = "grpc"
	s.TLS = "tls"
	s.Path = "grpc-service"
	m := parseCfg(t, s, testSettings())
	ss := obj(proxyOutbound(m), "streamSettings")
	grpc := obj(ss, "grpcSettings")
	if grpc["serviceName"] != "grpc-service" {
		t.Errorf("grpc serviceName=%v", grpc["serviceName"])
	}
	tls := obj(ss, "tlsSettings")
	if tls["serverName"] != "example.com" {
		t.Errorf("serverName 回退=%v, want example.com", tls["serverName"])
	}
}

func TestConfigWSNoHostOmitsHeaders(t *testing.T) {
	s := baseServer("vmess")
	s.Network = "ws"
	s.Path = "/v2"
	m := parseCfg(t, s, testSettings())
	ws := obj(obj(proxyOutbound(m), "streamSettings"), "wsSettings")
	if _, ok := ws["headers"]; ok {
		t.Error("Host 为空时不应输出 headers")
	}
}

// advancedSettings 构造开启全部高级字段的 Settings,用于 TestBuildConfigAdvanced。
func advancedSettings() Settings {
	return Settings{
		SocksPort:        10808,
		HTTPPort:         10809,
		RouteMode:        "rules",
		LogLevel:         "info",
		DomainStrategy:   "IPOnDemand",
		ListenAddress:    "0.0.0.0",
		SocksUDP:         true,
		SocksUser:        "user1",
		SocksPass:        "pass1",
		Sniffing:         true,
		SniffingOverride: "http,tls,quic",
		Mux:              true,
		MuxConcurrency:   4,
		TCPFastOpen:      true,
		TCPKeepAlive:     30,
		Fwmark:           255,
		ForceDNS:         true,
		DNSServers:       "https://dns.google/dns-query, 1.1.1.1",
		ProxyIgnore:      "localhost,127.0.0.0/8",
		LatencyTimeout:   5,
		TunEnable:        true,
		TunSubnet:        "10.0.0.1/24",
		TunMTU:           1400,
	}
}

// TestBuildConfigAdvanced 校验全部高级字段映射到 v4 配置。
func TestBuildConfigAdvanced(t *testing.T) {
	m := parseCfg(t, baseServer("vmess"), advancedSettings())

	// log 段:loglevel 使用字段值。
	if lv := obj(m, "log")["loglevel"]; lv != "info" {
		t.Errorf("loglevel=%v, want info", lv)
	}

	// 入站:监听地址、SOCKS 认证、UDP、sniffing。
	inbounds := arr(m, "inbounds")
	if len(inbounds) != 2 {
		t.Fatalf("inbounds=%d, want 2", len(inbounds))
	}
	socks := inbounds[0].(map[string]interface{})
	http := inbounds[1].(map[string]interface{})
	if socks["listen"] != "0.0.0.0" || http["listen"] != "0.0.0.0" {
		t.Errorf("listen 应为 0.0.0.0: socks=%v http=%v", socks["listen"], http["listen"])
	}
	socksSettings := obj(socks, "settings")
	if socksSettings["auth"] != "password" {
		t.Errorf("auth=%v, want password", socksSettings["auth"])
	}
	if socksSettings["udp"] != true {
		t.Errorf("udp=%v, want true", socksSettings["udp"])
	}
	accounts := arr(socksSettings, "accounts")
	acc := accounts[0].(map[string]interface{})
	if acc["user"] != "user1" || acc["pass"] != "pass1" {
		t.Errorf("accounts=%v", acc)
	}
	for i, in := range inbounds {
		ib := in.(map[string]interface{})
		sniff, ok := ib["sniffing"].(map[string]interface{})
		if !ok {
			t.Fatalf("inbounds[%d] 缺少 sniffing", i)
		}
		if sniff["enabled"] != true {
			t.Errorf("inbounds[%d] sniffing.enabled=%v", i, sniff["enabled"])
		}
		dest := arr(sniff, "destOverride")
		if len(dest) != 3 || dest[0] != "http" || dest[1] != "tls" || dest[2] != "quic" {
			t.Errorf("inbounds[%d] destOverride=%v, want [http tls quic]", i, dest)
		}
	}

	// 代理出站:mux 与 sockopt(mark/tcpFastOpen/tcpKeepAliveInterval)。
	out := proxyOutbound(m)
	mux := obj(out, "mux")
	if mux["enabled"] != true || mux["concurrency"] != float64(4) {
		t.Errorf("mux=%v", mux)
	}
	sockopt := obj(obj(out, "streamSettings"), "sockopt")
	if sockopt["tcpFastOpen"] != true {
		t.Errorf("tcpFastOpen=%v", sockopt["tcpFastOpen"])
	}
	if sockopt["tcpKeepAliveInterval"] != float64(30) {
		t.Errorf("tcpKeepAliveInterval=%v", sockopt["tcpKeepAliveInterval"])
	}
	if sockopt["mark"] != float64(255) {
		t.Errorf("mark=%v", sockopt["mark"])
	}

	// dns 段:逗号拆分并 trim。
	dnsServers := arr(obj(m, "dns"), "servers")
	if len(dnsServers) != 2 ||
		dnsServers[0] != "https://dns.google/dns-query" || dnsServers[1] != "1.1.1.1" {
		t.Errorf("dns.servers=%v", dnsServers)
	}

	// routing:domainStrategy 用字段值;ForceDNS 规则在最前。
	routing := obj(m, "routing")
	if routing["domainStrategy"] != "IPOnDemand" {
		t.Errorf("domainStrategy=%v", routing["domainStrategy"])
	}
	rules := arr(routing, "rules")
	if len(rules) < 1 {
		t.Fatal("缺少路由规则")
	}
	force := rules[0].(map[string]interface{})
	if force["port"] != "53,853" || force["network"] != "tcp,udp" || force["outboundTag"] != "proxy" {
		t.Errorf("ForceDNS 规则=%v", force)
	}

	// services.tun:jsonpb snake_case 字段、base64 的 ip/prefix、固定 0.0.0.0/0 路由。
	services := obj(m, "services")
	tun := obj(services, "tun")
	if tun["name"] != "tun0" {
		t.Errorf("tun.name=%v", tun["name"])
	}
	if tun["mtu"] != float64(1400) {
		t.Errorf("tun.mtu=%v, want 1400", tun["mtu"])
	}
	if tun["packet_encoding"] != "Packet" {
		t.Errorf("tun.packet_encoding=%v", tun["packet_encoding"])
	}
	if tun["tag"] != "tun-in" {
		t.Errorf("tun.tag=%v", tun["tag"])
	}
	ips := arr(tun, "ips")
	ip0 := ips[0].(map[string]interface{})
	if ip0["ip"] != "CgAAAQ==" || ip0["prefix"] != float64(24) {
		t.Errorf("tun.ips[0]=%v, want ip=CgAAAQ== prefix=24", ip0)
	}
	routes := arr(tun, "routes")
	rt0 := routes[0].(map[string]interface{})
	if rt0["ip"] != "AAAAAA==" || rt0["prefix"] != float64(0) {
		t.Errorf("tun.routes[0]=%v, want ip=AAAAAA== prefix=0", rt0)
	}
	sniff := obj(tun, "sniffing_settings")
	if sniff["enabled"] != true {
		t.Errorf("tun.sniffing_settings.enabled=%v", sniff["enabled"])
	}
	dest := arr(sniff, "dest_override")
	if len(dest) != 2 || dest[0] != "http" || dest[1] != "tls" {
		t.Errorf("tun.sniffing_settings.dest_override=%v", dest)
	}
}

// TestBuildConfigAdvancedZeroCompat 校验零值 Settings 保持旧输出(loglevel warning、
// listen 127.0.0.1、udp true、无 sniffing/mux/sockopt/dns/tun 段)。
func TestBuildConfigAdvancedZeroCompat(t *testing.T) {
	m := parseCfg(t, baseServer("vmess"), Settings{}) // 全零值
	if lv := obj(m, "log")["loglevel"]; lv != "warning" {
		t.Errorf("loglevel=%v, want warning", lv)
	}
	socks := arr(m, "inbounds")[0].(map[string]interface{})
	if socks["listen"] != "127.0.0.1" {
		t.Errorf("listen=%v, want 127.0.0.1", socks["listen"])
	}
	if udp := obj(socks, "settings")["udp"]; udp != true {
		t.Errorf("零值 Settings 的 udp 应保持 true,got %v", udp)
	}
	if _, ok := socks["sniffing"]; ok {
		t.Error("Sniffing 关闭时不应输出 sniffing 段")
	}
	out := proxyOutbound(m)
	if _, ok := out["mux"]; ok {
		t.Error("Mux 关闭时不应输出 mux 段")
	}
	if _, ok := obj(out, "streamSettings")["sockopt"]; ok {
		t.Error("sockopt 全部零值时不应输出 sockopt 段")
	}
	if _, ok := m["dns"]; ok {
		t.Error("DNSServers 为空时不应输出 dns 段")
	}
	if _, ok := m["services"]; ok {
		t.Error("TunEnable 关闭时不应输出 services 段")
	}
	rules := arr(obj(m, "routing"), "rules")
	if rules[0].(map[string]interface{})["port"] != nil {
		t.Error("ForceDNS 关闭时不应有强制 DNS 规则")
	}
}

// TestTunCIDREncode 校验 cidrToIPBase64 的 base64 编码与前缀。
func TestTunCIDREncode(t *testing.T) {
	ip, prefix, err := cidrToIPBase64("10.0.0.1/24")
	if err != nil {
		t.Fatalf("cidrToIPBase64(10.0.0.1/24): %v", err)
	}
	if ip != "CgAAAQ==" || prefix != 24 {
		t.Errorf("got (%s, %d), want (CgAAAQ==, 24)", ip, prefix)
	}
	ip, prefix, err = cidrToIPBase64("0.0.0.0/0")
	if err != nil {
		t.Fatalf("cidrToIPBase64(0.0.0.0/0): %v", err)
	}
	if ip != "AAAAAA==" || prefix != 0 {
		t.Errorf("got (%s, %d), want (AAAAAA==, 0)", ip, prefix)
	}
	if _, _, err := cidrToIPBase64("not-a-cidr"); err == nil {
		t.Error("非法 CIDR 应返回错误")
	}
}
