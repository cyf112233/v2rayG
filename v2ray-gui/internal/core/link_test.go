package core

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// vmessLink 生成 vmess://<base64 JSON> 形式的链接,ps 为节点名。
func vmessLink(ps string, m map[string]interface{}) string {
	if m == nil {
		m = map[string]interface{}{}
	}
	m["v"] = "2"
	m["ps"] = ps
	data, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(data)
}

func TestParseVmessBase64(t *testing.T) {
	link := vmessLink("测试节点", map[string]interface{}{
		"add": "1.2.3.4", "port": 443, "id": "uuid-1", "aid": 64,
		"net": "ws", "type": "none", "host": "h.example.com", "path": "/ws",
		"tls": "tls", "sni": "sni.example.com", "alpn": "h2,http/1.1", "fp": "chrome",
	})
	s, err := ParseLink(link)
	if err != nil {
		t.Fatalf("ParseLink: %v", err)
	}
	if s.Protocol != "vmess" {
		t.Errorf("protocol=%s, want vmess", s.Protocol)
	}
	if s.Name != "测试节点" {
		t.Errorf("name=%q, want 测试节点", s.Name)
	}
	if s.Address != "1.2.3.4" || s.Port != 443 {
		t.Errorf("addr/port=%s:%d", s.Address, s.Port)
	}
	if s.UUID != "uuid-1" || s.AlterID != 64 {
		t.Errorf("uuid/alterId=%s/%d", s.UUID, s.AlterID)
	}
	if s.Network != "ws" || s.Host != "h.example.com" || s.Path != "/ws" {
		t.Errorf("net/host/path=%s/%s/%s", s.Network, s.Host, s.Path)
	}
	if s.TLS != "tls" || s.SNI != "sni.example.com" {
		t.Errorf("tls/sni=%s/%s", s.TLS, s.SNI)
	}
	if len(s.ALPN) != 2 || s.ALPN[0] != "h2" || s.ALPN[1] != "http/1.1" {
		t.Errorf("alpn=%v", s.ALPN)
	}
	if s.Fingerprint != "chrome" {
		t.Errorf("fp=%s", s.Fingerprint)
	}
}

func TestParseVmessPortAsString(t *testing.T) {
	// port 兼容字符串;tls 空 → "none"。
	link := vmessLink("", map[string]interface{}{"add": "x.com", "port": "8443", "id": "u", "net": "tcp", "tls": ""})
	s, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if s.Port != 8443 {
		t.Errorf("port=%d, want 8443", s.Port)
	}
	if s.TLS != "none" {
		t.Errorf("tls=%q, want none", s.TLS)
	}
	if s.Name == "" {
		t.Error("Name 为空时应回退为 host:port")
	}
}

func TestParseVmessUserInfo(t *testing.T) {
	link := "vmess://uuid-9@example.com:8443?security=auto&tls=tls&type=ws&host=ws.example.com&path=%2Fws&fp=chrome#节点A"
	s, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "节点A" || s.Address != "example.com" || s.Port != 8443 || s.UUID != "uuid-9" {
		t.Errorf("基础字段: %+v", s)
	}
	if s.Security != "auto" || s.TLS != "tls" || s.Network != "ws" ||
		s.Host != "ws.example.com" || s.Path != "/ws" || s.Fingerprint != "chrome" {
		t.Errorf("query 字段: %+v", s)
	}
}

func TestParseVlessReality(t *testing.T) {
	link := "vless://uuid-r@r.example.com:443?encryption=none&security=reality&type=tcp&sni=r.example.com&fp=chrome&pbk=pk&sid=sid1&flow=xtls-rprx-vision#Real节点"
	s, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if s.Protocol != "vless" || s.TLS != "reality" {
		t.Errorf("protocol/tls=%s/%s", s.Protocol, s.TLS)
	}
	if s.PublicKey != "pk" || s.ShortID != "sid1" {
		t.Errorf("reality pk/sid=%s/%s", s.PublicKey, s.ShortID)
	}
	if s.Flow != "xtls-rprx-vision" || s.Encryption != "none" || s.Fingerprint != "chrome" {
		t.Errorf("flow/enc/fp=%s/%s/%s", s.Flow, s.Encryption, s.Fingerprint)
	}
	if s.SNI != "r.example.com" {
		t.Errorf("sni=%s", s.SNI)
	}
}

func TestParseVlessWS(t *testing.T) {
	link := "vless://uuid-w@w.example.com:443?encryption=none&security=tls&type=ws&host=cdn.example.com&path=%2Fv2&sni=cdn.example.com&alpn=h2,http%2F1.1#WS节点"
	s, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if s.Network != "ws" || s.Host != "cdn.example.com" || s.Path != "/v2" {
		t.Errorf("ws 字段: %+v", s)
	}
	if s.TLS != "tls" || s.SNI != "cdn.example.com" {
		t.Errorf("tls 字段: %+v", s)
	}
	if len(s.ALPN) != 2 || s.ALPN[1] != "http/1.1" {
		t.Errorf("alpn=%v", s.ALPN)
	}
	if s.Name != "WS节点" {
		t.Errorf("name=%q", s.Name)
	}
}

func TestParseTrojan(t *testing.T) {
	link := "trojan://pass123@t.example.com:443?security=tls&type=ws&host=t.example.com&path=%2F&sni=t.example.com&alpn=h2&fp=chrome&flow=xtls-rprx-vision#Trojan节点"
	s, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if s.Protocol != "trojan" || s.UUID != "pass123" {
		t.Errorf("protocol/uuid=%s/%s", s.Protocol, s.UUID)
	}
	if s.TLS != "tls" || s.Network != "ws" || s.Host != "t.example.com" {
		t.Errorf("tls/net/host=%s/%s/%s", s.TLS, s.Network, s.Host)
	}
	if s.Flow != "xtls-rprx-vision" {
		t.Errorf("flow=%s", s.Flow)
	}
	if s.Name != "Trojan节点" {
		t.Errorf("name=%q", s.Name)
	}
}

func TestParseSSFullBase64(t *testing.T) {
	// 形式 1: 整体 base64,内容是 method:password@host:port。
	plain := "aes-256-gcm:pass123@ss.example.com:8388"
	link := "ss://" + base64.StdEncoding.EncodeToString([]byte(plain)) + "#SS节点"
	s, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if s.Protocol != "shadowsocks" || s.Method != "aes-256-gcm" || s.Password != "pass123" {
		t.Errorf("ss 字段: %+v", s)
	}
	if s.Address != "ss.example.com" || s.Port != 8388 {
		t.Errorf("addr/port=%s:%d", s.Address, s.Port)
	}
	if s.Name != "SS节点" {
		t.Errorf("name=%q", s.Name)
	}
}

func TestParseSSPlainUserInfo(t *testing.T) {
	// 形式 2: 明文 method:password@host:port#name。
	link := "ss://chacha20-ietf-poly1305:secret@ss2.example.com:8443#明文"
	s, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if s.Method != "chacha20-ietf-poly1305" || s.Password != "secret" {
		t.Errorf("ss 字段: %+v", s)
	}
	if s.Address != "ss2.example.com" || s.Port != 8443 || s.Name != "明文" {
		t.Errorf("addr/port/name=%s:%d/%q", s.Address, s.Port, s.Name)
	}
}

func TestParseSSBase64UserInfo(t *testing.T) {
	// 形式 3: base64(method:password)@host:port#name。
	userinfo := base64.StdEncoding.EncodeToString([]byte("aes-128-gcm:p@ss"))
	link := "ss://" + userinfo + "@ss3.example.com:80#b64user"
	s, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if s.Method != "aes-128-gcm" || s.Password != "p@ss" {
		t.Errorf("ss 字段: %+v", s)
	}
	if s.Address != "ss3.example.com" || s.Port != 80 {
		t.Errorf("addr/port=%s:%d", s.Address, s.Port)
	}
}

func TestParseInvalid(t *testing.T) {
	bad := []string{
		"不是链接",
		"http://example.com",
		"ssr://xxxx",
		"vmess://%%%",
		"vless://",
	}
	for _, line := range bad {
		if _, err := ParseLink(line); err == nil {
			t.Errorf("ParseLink(%q) 应返回错误", line)
		}
	}
	if _, err := ParseLink("ssr://xxxx"); err == nil || !strings.Contains(err.Error(), "不支持") {
		t.Errorf("ssr 应报『不支持的链接类型』,got: %v", err)
	}
}

func TestServerKey(t *testing.T) {
	a := &Server{Protocol: "vmess", Address: "a.com", Port: 443, UUID: "u"}
	b := &Server{Protocol: "vmess", Address: "a.com", Port: 443, UUID: "u"}
	c := &Server{Protocol: "vmess", Address: "a.com", Port: 443, UUID: "v"}
	if a.Key() != b.Key() {
		t.Error("相同节点 Key 应一致")
	}
	if a.Key() == c.Key() {
		t.Error("不同 UUID 的 Key 应不同")
	}
}

func TestNewIDFormat(t *testing.T) {
	id := NewID()
	if len(id) != 36 || id[14] != '4' {
		t.Errorf("UUID v4 格式不正确: %q", id)
	}
	if NewID() == NewID() {
		t.Error("两次生成的 ID 不应相同")
	}
}
