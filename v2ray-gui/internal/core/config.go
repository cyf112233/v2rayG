package core

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"strings"
)

// BuildV2rayConfig 根据服务器与设置生成 v4 传统格式的 v2ray JSON 配置。
// 配置包含 "stats" 与 "policy" 以启用流量统计;路由按 RouteMode 生成:
//   - rules(默认):私有网段直连,其余走代理,不依赖 geoip 资产文件;
//   - global:无路由规则,全部流量走第一个出站(代理);
//   - direct:全匹配兜底规则,全部流量直连。
//
// 高级设置(LogLevel/DomainStrategy/ListenAddress/Sniffing/Mux/sockopt/DNS/
// 强制 DNS/TUN)全部按 Settings 生成,零值字段回退到默认值。
//
// reality 服务器需要 Xray-core,当前 v2fly 核心不支持,返回错误。
func BuildV2rayConfig(s *Server, st Settings) ([]byte, error) {
	if s.TLS == "reality" {
		return nil, errors.New("Reality 协议需要 Xray-core,当前 v2fly 核心不支持")
	}
	// socks udp:显式配置时用字段值;零值 Settings(SocksPort==0 且未开启)保持旧行为 true。
	// 必须在 SocksPort 默认值填充之前计算。
	udp := st.SocksUDP
	if st.SocksPort == 0 && !st.SocksUDP {
		udp = true
	}
	if st.RouteMode == "" {
		st.RouteMode = "rules" // 兼容旧配置
	}
	if st.SocksPort == 0 {
		st.SocksPort = 10808
	}
	if st.HTTPPort == 0 {
		st.HTTPPort = 10809
	}
	if st.LogLevel == "" {
		st.LogLevel = "warning"
	}
	if st.DomainStrategy == "" {
		st.DomainStrategy = "IPIfNonMatch"
	}
	if st.ListenAddress == "" {
		st.ListenAddress = "127.0.0.1"
	}
	if st.SniffingOverride == "" {
		st.SniffingOverride = "http,tls"
	}
	if st.MuxConcurrency == 0 {
		st.MuxConcurrency = 8
	}
	if st.TunSubnet == "" {
		st.TunSubnet = "10.0.0.1/24"
	}
	if st.TunMTU == 0 {
		st.TunMTU = 1500
	}
	if st.ProxyIgnore == "" {
		st.ProxyIgnore = "localhost,127.0.0.0/8,::1"
	}

	proxy, err := outboundProxy(s)
	if err != nil {
		return nil, err
	}
	// 代理出站:按需附加 sockopt 与 mux(挂在 streamSettings 之上)。
	applyAdvancedOutbound(proxy, st)

	inbounds := []interface{}{
		map[string]interface{}{
			"tag":      "socks-in",
			"listen":   st.ListenAddress,
			"port":     st.SocksPort,
			"protocol": "socks",
			"settings": socksSettings(st, udp),
		},
		map[string]interface{}{
			"tag":      "http-in",
			"listen":   st.ListenAddress,
			"port":     st.HTTPPort,
			"protocol": "http",
			"settings": map[string]interface{}{},
		},
	}
	// 两个入站都加 sniffing(仅当开启)。
	if st.Sniffing {
		sniff := map[string]interface{}{
			"enabled":      true,
			"destOverride": splitComma(st.SniffingOverride),
		}
		inbounds[0].(map[string]interface{})["sniffing"] = sniff
		inbounds[1].(map[string]interface{})["sniffing"] = sniff
	}

	cfg := map[string]interface{}{
		"log":   map[string]interface{}{"loglevel": st.LogLevel},
		"stats": map[string]interface{}{},
		"policy": map[string]interface{}{
			"system": map[string]interface{}{
				"statsInboundUplink":   true,
				"statsInboundDownlink": true,
			},
		},
		"inbounds": inbounds,
		"outbounds": []interface{}{
			proxy,
			map[string]interface{}{
				"protocol": "freedom",
				"tag":      "direct",
				"settings": map[string]interface{}{},
			},
		},
		"routing": map[string]interface{}{
			"domainStrategy": st.DomainStrategy,
			"rules":          routingRulesWithForceDNS(st),
		},
	}
	// 根级 dns 段(仅当配置了 DNS 服务器)。
	if dnsServers := splitComma(st.DNSServers); len(dnsServers) > 0 {
		cfg["dns"] = map[string]interface{}{"servers": dnsServers}
	}
	// 根级 services.tun(仅当开启 TUN)。
	if st.TunEnable {
		tun, err := tunServices(st)
		if err != nil {
			return nil, err
		}
		cfg["services"] = map[string]interface{}{"tun": tun}
	}
	return json.MarshalIndent(cfg, "", "  ")
}

// socksSettings 生成 SOCKS 入站 settings:SocksUser 非空时启用密码认证。
func socksSettings(st Settings, udp bool) map[string]interface{} {
	settings := map[string]interface{}{
		"auth": "noauth",
		"udp":  udp,
	}
	if st.SocksUser != "" {
		settings["auth"] = "password"
		settings["accounts"] = []interface{}{
			map[string]interface{}{
				"user": st.SocksUser,
				"pass": st.SocksPass,
			},
		}
	}
	return settings
}

// applyAdvancedOutbound 在代理出站上附加 mux 与 streamSettings.sockopt。
// mux 挂在出站级;tcpFastOpen 是 bool,mark/tcpKeepAliveInterval 是数字;
// sockopt 只有任一条件成立才输出。
func applyAdvancedOutbound(out map[string]interface{}, st Settings) {
	if st.Mux {
		out["mux"] = map[string]interface{}{
			"enabled":     true,
			"concurrency": st.MuxConcurrency,
		}
	}
	sockopt := map[string]interface{}{}
	if st.TCPFastOpen {
		sockopt["tcpFastOpen"] = true
	}
	if st.TCPKeepAlive > 0 {
		sockopt["tcpKeepAliveInterval"] = st.TCPKeepAlive
	}
	if st.Fwmark > 0 {
		sockopt["mark"] = st.Fwmark
	}
	if len(sockopt) > 0 {
		ss, ok := out["streamSettings"].(map[string]interface{})
		if !ok {
			return
		}
		ss["sockopt"] = sockopt
	}
}

// tunServices 生成根级 services.tun 配置(jsonpb 解析,snake_case 字段)。
// ips 用 st.TunSubnet 解析(IPv4 4 字节 To4() base64),routes 固定 0.0.0.0/0。
// st.TunFD>0 时附加 preopened_fd:核心直接用该 fd 创建设备,无需自身提权。
func tunServices(st Settings) (map[string]interface{}, error) {
	ipBase64, prefix, err := cidrToIPBase64(st.TunSubnet)
	if err != nil {
		return nil, err
	}
	tun := map[string]interface{}{
		"name":            TunName,
		"mtu":             st.TunMTU,
		"user_level":      0,
		"packet_encoding": "Packet",
		"tag":             "tun-in",
		"ips": []interface{}{
			map[string]interface{}{
				"ip":     ipBase64,
				"prefix": prefix,
			},
		},
		"routes": []interface{}{
			map[string]interface{}{
				"ip":     "AAAAAA==", // 0.0.0.0
				"prefix": 0,
			},
		},
		"sniffing_settings": map[string]interface{}{
			"enabled":              true,
			"destination_override": []interface{}{"http", "tls"},
		},
	}
	if st.TunFD > 0 {
		tun["preopened_fd"] = st.TunFD
	}
	return tun, nil
}

// cidrToIPBase64 将 IPv4 网段(如 "10.0.0.1/24")编码为
// {"ip": base64(4 字节 To4()), "prefix": N},与 services.tun.ips 的 jsonpb 格式一致。
func cidrToIPBase64(cidr string) (ipBase64 string, prefix int, err error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", 0, err
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return "", 0, errors.New("仅支持 IPv4 网段: " + cidr)
	}
	ones, _ := ipNet.Mask.Size()
	return base64.StdEncoding.EncodeToString(ip4), ones, nil
}

// splitComma 将逗号分隔的文本拆为 trim 后的非空字符串列表。
func splitComma(s string) []string {
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

// routingRulesWithForceDNS 生成路由规则:开启强制 DNS 且模式为 rules/global 时,
// 最前面追加一条把 53/853 端口(tcp/udp)强制走代理的规则。
func routingRulesWithForceDNS(st Settings) []interface{} {
	forceDNS := []interface{}{
		map[string]interface{}{
			"type":        "field",
			"port":        "53,853",
			"network":     "tcp,udp",
			"outboundTag": "proxy",
		},
	}
	if st.ForceDNS && (st.RouteMode == "rules" || st.RouteMode == "global") {
		return append(forceDNS, routingRules(st.RouteMode)...)
	}
	return routingRules(st.RouteMode)
}

// routingRules 按路由模式生成路由规则列表:
//   - "rules"(默认):私有网段直连(127/8、10/8、172.16/12、192.168/16、100.64/10、::1、fc00::/7、fe80::/10),
//     其余流量落到兜底出站(第一个出站即代理);
//   - "global":空规则,默认全部走第一个出站(代理);
//   - "direct":显式全匹配兜底规则(用 network 条件避免空规则不被匹配的歧义),全部直连。
func routingRules(mode string) []interface{} {
	switch mode {
	case "global":
		return []interface{}{}
	case "direct":
		return []interface{}{
			map[string]interface{}{
				"type":        "field",
				"network":     "tcp,udp",
				"outboundTag": "direct",
			},
		}
	default: // "rules"
		return []interface{}{
			map[string]interface{}{
				"type": "field",
				"ip": []interface{}{
					"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
					"100.64.0.0/10", "::1/128", "fc00::/7", "fe80::/10",
				},
				"outboundTag": "direct",
			},
		}
	}
}

// outboundProxy 按协议生成代理出站配置。
func outboundProxy(s *Server) (map[string]interface{}, error) {
	switch s.Protocol {
	case "vmess":
		return vmessOutbound(s), nil
	case "vless":
		return vlessOutbound(s), nil
	case "trojan":
		return trojanOutbound(s), nil
	case "shadowsocks":
		return ssOutbound(s), nil
	default:
		return nil, errors.New("不支持的协议: " + s.Protocol)
	}
}

// vmessOutbound 生成 vmess 出站:settings.vnext[].users[]={id,alterId,security}。
func vmessOutbound(s *Server) map[string]interface{} {
	sec := s.Security
	if sec == "" {
		sec = "auto"
	}
	return map[string]interface{}{
		"protocol": "vmess",
		"tag":      "proxy",
		"settings": map[string]interface{}{
			"vnext": []interface{}{
				map[string]interface{}{
					"address": s.Address,
					"port":    s.Port,
					"users": []interface{}{
						map[string]interface{}{
							"id":       s.UUID,
							"alterId":  s.AlterID,
							"security": sec,
						},
					},
				},
			},
		},
		"streamSettings": streamSettings(s),
	}
}

// vlessOutbound 生成 vless 出站:vnext[].users[]={id,encryption,flow}。
func vlessOutbound(s *Server) map[string]interface{} {
	enc := s.Encryption
	if enc == "" {
		enc = "none"
	}
	user := map[string]interface{}{"id": s.UUID, "encryption": enc}
	if s.Flow != "" {
		user["flow"] = s.Flow
	}
	return map[string]interface{}{
		"protocol": "vless",
		"tag":      "proxy",
		"settings": map[string]interface{}{
			"vnext": []interface{}{
				map[string]interface{}{
					"address": s.Address,
					"port":    s.Port,
					"users":   []interface{}{user},
				},
			},
		},
		"streamSettings": streamSettings(s),
	}
}

// trojanOutbound 生成 trojan 出站:servers[]={address,port,password,level,flow?}。
func trojanOutbound(s *Server) map[string]interface{} {
	server := map[string]interface{}{
		"address":  s.Address,
		"port":     s.Port,
		"password": s.UUID,
		"level":    0,
	}
	if s.Flow != "" {
		server["flow"] = s.Flow
	}
	return map[string]interface{}{
		"protocol": "trojan",
		"tag":      "proxy",
		"settings": map[string]interface{}{
			"servers": []interface{}{server},
		},
		"streamSettings": streamSettings(s),
	}
}

// ssOutbound 生成 shadowsocks 出站:servers[]={address,port,method,password,level}。
func ssOutbound(s *Server) map[string]interface{} {
	return map[string]interface{}{
		"protocol": "shadowsocks",
		"tag":      "proxy",
		"settings": map[string]interface{}{
			"servers": []interface{}{
				map[string]interface{}{
					"address":  s.Address,
					"port":     s.Port,
					"method":   s.Method,
					"password": s.Password,
					"level":    0,
				},
			},
		},
		"streamSettings": streamSettings(s),
	}
}

// streamSettings 生成传输层配置,network 默认 tcp;TLS 为 none 时不输出 security 字段。
func streamSettings(s *Server) map[string]interface{} {
	network := s.Network
	if network == "" {
		network = "tcp"
	}
	ss := map[string]interface{}{"network": network}
	switch network {
	case "ws":
		ws := map[string]interface{}{"path": s.Path}
		if s.Host != "" {
			ws["headers"] = map[string]interface{}{"Host": s.Host}
		}
		ss["wsSettings"] = ws
	case "grpc":
		ss["grpcSettings"] = map[string]interface{}{"serviceName": s.Path}
	case "h2":
		hosts := make([]interface{}, 0, 1)
		if s.Host != "" {
			hosts = append(hosts, s.Host)
		}
		ss["httpSettings"] = map[string]interface{}{"host": hosts, "path": s.Path}
	case "kcp":
		ss["kcpSettings"] = map[string]interface{}{"header": map[string]interface{}{"type": "none"}}
	case "quic":
		ss["quicSettings"] = map[string]interface{}{
			"security": "none",
			"header":   map[string]interface{}{"type": "none"},
		}
	}
	if s.TLS == "tls" {
		serverName := s.SNI
		if serverName == "" {
			serverName = s.Host
		}
		if serverName == "" {
			serverName = s.Address
		}
		tls := map[string]interface{}{
			"serverName":    serverName,
			"allowInsecure": true,
		}
		if len(s.ALPN) > 0 {
			alpn := make([]interface{}, 0, len(s.ALPN))
			for _, a := range s.ALPN {
				alpn = append(alpn, a)
			}
			tls["alpn"] = alpn
		}
		if s.Fingerprint != "" {
			tls["fingerprint"] = s.Fingerprint
		}
		ss["security"] = "tls"
		ss["tlsSettings"] = tls
	}
	return ss
}
