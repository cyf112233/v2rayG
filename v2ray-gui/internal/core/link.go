package core

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ParseLink 解析各类代理分享链接,返回服务器节点。
// 支持 vmess://、vless://、trojan://、ss:// 四种前缀。
func ParseLink(line string) (*Server, error) {
	line = strings.TrimSpace(line)
	idx := strings.Index(line, "://")
	if idx <= 0 {
		return nil, errors.New("不是有效的分享链接")
	}
	prefix := strings.ToLower(line[:idx])
	rest := line[idx+3:]
	switch prefix {
	case "vmess":
		return parseVmess(rest)
	case "vless":
		return parseVless(rest)
	case "trojan":
		return parseTrojan(rest)
	case "ss":
		return parseSS(rest)
	default:
		return nil, errors.New("不支持的链接类型: " + prefix)
	}
}

// base64Decode 依次尝试 RawURLEncoding 与 StdEncoding,容错解码。
func base64Decode(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	encs := []*base64.Encoding{base64.RawURLEncoding, base64.StdEncoding}
	var lastErr error
	for _, enc := range encs {
		data, err := enc.DecodeString(s)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// linkInfo 保存解析 vless/trojan/vmess(uuid@host) 形式的通用信息。
type linkInfo struct {
	user     string
	host     string
	port     int
	query    url.Values
	fragment string
}

// parseUserInfo 解析 vless://uuid@host:port?query#name 形式的链接主体。
func parseUserInfo(rest string) (*linkInfo, error) {
	u, err := url.Parse("x://" + rest)
	if err != nil {
		return nil, errors.New("链接解析失败: " + err.Error())
	}
	info := &linkInfo{
		user:     u.User.Username(),
		host:     u.Hostname(),
		query:    u.Query(),
		fragment: u.Fragment,
	}
	portStr := u.Port()
	if portStr == "" {
		portStr = "443"
	}
	info.port, err = strconv.Atoi(portStr)
	if err != nil {
		return nil, errors.New("端口无效: " + portStr)
	}
	if info.host == "" {
		return nil, errors.New("缺少服务器地址")
	}
	return info, nil
}

// newServer 创建带默认值的服务器节点。
func newServer(protocol string) *Server {
	return &Server{
		ID:        NewID(),
		Protocol:  protocol,
		LatencyMS: -1,
	}
}

// parseVmess 解析 vmess:// 链接,支持 base64 JSON 与 uuid@host 两种形式。
func parseVmess(rest string) (*Server, error) {
	if strings.Contains(rest, "@") {
		return parseVmessUserInfo(rest)
	}
	// 形式 1: vmess://<base64>,base64 内容是 JSON。
	data, err := base64Decode(rest)
	if err != nil {
		return nil, errors.New("vmess base64 解码失败: " + err.Error())
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, errors.New("vmess JSON 解析失败: " + err.Error())
	}
	s := newServer("vmess")
	s.Name = strField(m["ps"])
	s.Address = strField(m["add"])
	s.Port = intField(m["port"])
	s.UUID = strField(m["id"])
	s.AlterID = intField(m["aid"])
	s.Security = strField(m["scy"])
	net := strField(m["net"])
	if net == "" {
		net = strField(m["type"])
	}
	s.Network = normNetwork(net)
	s.TLS = normTLS(strField(m["tls"]))
	s.Host = strField(m["host"])
	s.Path = strField(m["path"])
	s.SNI = strField(m["sni"])
	s.Flow = strField(m["flow"])
	s.Fingerprint = strField(m["fp"])
	s.ALPN = alpnField(m["alpn"])
	if s.Name == "" {
		s.Name = hostPortName(s.Address, s.Port)
	}
	return s, nil
}

// parseVmessUserInfo 解析 vmess://uuid@host:port?query#name 形式。
func parseVmessUserInfo(rest string) (*Server, error) {
	info, err := parseUserInfo(rest)
	if err != nil {
		return nil, err
	}
	s := newServer("vmess")
	s.Address = info.host
	s.Port = info.port
	s.UUID = info.user
	s.Security = info.query.Get("security")
	s.TLS = normTLS(info.query.Get("tls"))
	s.Network = normNetwork(info.query.Get("type"))
	s.Host = info.query.Get("host")
	s.Path = info.query.Get("path")
	s.SNI = info.query.Get("sni")
	s.Fingerprint = info.query.Get("fp")
	s.Flow = info.query.Get("flow")
	s.ALPN = splitList(info.query.Get("alpn"))
	s.Name = linkName(info.fragment, s.Address, s.Port)
	return s, nil
}

// parseVless 解析 vless:// 链接。
func parseVless(rest string) (*Server, error) {
	info, err := parseUserInfo(rest)
	if err != nil {
		return nil, err
	}
	s := newServer("vless")
	s.Address = info.host
	s.Port = info.port
	s.UUID = info.user
	s.Encryption = info.query.Get("encryption")
	if s.Encryption == "" {
		s.Encryption = "none"
	}
	s.TLS = normTLS(info.query.Get("security"))
	s.Network = normNetwork(info.query.Get("type"))
	s.Host = info.query.Get("host")
	s.Path = info.query.Get("path")
	s.SNI = info.query.Get("sni")
	s.Fingerprint = info.query.Get("fp")
	s.Flow = info.query.Get("flow")
	s.PublicKey = info.query.Get("pbk")
	s.ShortID = info.query.Get("sid")
	s.ALPN = splitList(info.query.Get("alpn"))
	s.Name = linkName(info.fragment, s.Address, s.Port)
	return s, nil
}

// parseTrojan 解析 trojan:// 链接,密码存入 UUID 字段。
func parseTrojan(rest string) (*Server, error) {
	info, err := parseUserInfo(rest)
	if err != nil {
		return nil, err
	}
	s := newServer("trojan")
	s.Address = info.host
	s.Port = info.port
	s.UUID = info.user
	s.TLS = normTLS(info.query.Get("security"))
	s.Network = normNetwork(info.query.Get("type"))
	s.Host = info.query.Get("host")
	s.Path = info.query.Get("path")
	s.SNI = info.query.Get("sni")
	s.Fingerprint = info.query.Get("fp")
	s.Flow = info.query.Get("flow")
	s.ALPN = splitList(info.query.Get("alpn"))
	s.Name = linkName(info.fragment, s.Address, s.Port)
	return s, nil
}

// parseSS 解析 ss:// 链接,支持整体 base64、明文、base64(method:password) 三种形式。
func parseSS(rest string) (*Server, error) {
	fragment := ""
	if i := strings.Index(rest, "#"); i >= 0 {
		fragment = rest[i+1:]
		rest = rest[:i]
	}
	s := newServer("shadowsocks")
	var method, password, hostport string
	if strings.Contains(rest, "@") {
		// 形式 2/3: userinfo@host:port,userinfo 可能是明文或 base64。
		at := strings.Index(rest, "@")
		userinfo, host := rest[:at], rest[at+1:]
		if strings.Contains(userinfo, ":") {
			method, password = splitMethodPassword(userinfo)
		} else {
			data, err := base64Decode(userinfo)
			if err != nil {
				return nil, errors.New("ss userinfo 解码失败: " + err.Error())
			}
			method, password = splitMethodPassword(string(data))
		}
		hostport = host
	} else {
		// 形式 1: 整体 base64,内容是 method:password@host:port。
		data, err := base64Decode(rest)
		if err != nil {
			return nil, errors.New("ss base64 解码失败: " + err.Error())
		}
		plain := strings.TrimSpace(string(data))
		parts := strings.SplitN(plain, "@", 2)
		if len(parts) != 2 {
			return nil, errors.New("ss 链接内容格式错误")
		}
		method, password = splitMethodPassword(parts[0])
		hostport = parts[1]
	}
	host, port, err := splitHostPort(hostport)
	if err != nil {
		return nil, err
	}
	s.Method = method
	s.Password = password
	s.Address = host
	s.Port = port
	s.Name = linkName(fragment, s.Address, s.Port)
	return s, nil
}

// splitMethodPassword 将 "method:password" 拆分为加密方式与密码。
func splitMethodPassword(s string) (string, string) {
	i := strings.Index(s, ":")
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

// splitHostPort 拆分 "host:port",支持 IPv6 方括号形式。
func splitHostPort(hostport string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		i := strings.LastIndex(hostport, ":")
		if i < 0 {
			return "", 0, errors.New("缺少端口: " + hostport)
		}
		host, portStr = hostport[:i], hostport[i+1:]
	}
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, errors.New("端口无效: " + portStr)
	}
	if host == "" {
		return "", 0, errors.New("缺少服务器地址")
	}
	return host, port, nil
}

// normNetwork 归一化传输协议名称。
func normNetwork(n string) string {
	switch n {
	case "ws", "websocket":
		return "ws"
	case "grpc":
		return "grpc"
	case "h2", "http":
		return "h2"
	case "kcp", "mkcp":
		return "kcp"
	case "quic":
		return "quic"
	default:
		return "tcp"
	}
}

// normTLS 归一化 TLS 字段:空或 "none" 记 none,其余原样保存。
func normTLS(t string) string {
	if t == "" || t == "none" {
		return "none"
	}
	return t
}

// linkName 由 fragment 或 host:port 生成服务器名称。
func linkName(fragment, host string, port int) string {
	if fragment != "" {
		return fragment
	}
	return hostPortName(host, port)
}

func hostPortName(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// strField 将 JSON 值转为字符串。
func strField(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprint(t)
	}
}

// intField 将 JSON 值转为整数,支持字符串与数字。
func intField(v interface{}) int {
	switch t := v.(type) {
	case nil:
		return 0
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

// alpnField 将 ALPN 值转为字符串切片,支持字符串与 JSON 数组。
func alpnField(v interface{}) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, a := range t {
			if s, ok := a.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return splitList(strField(v))
	}
}

// splitList 将逗号分隔的字符串转为切片,过滤空项。
func splitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
