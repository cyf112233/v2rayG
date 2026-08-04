// Package core 提供服务器数据模型、链接解析、v4 配置生成、延迟测试与系统代理。
package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Server 表示一个代理服务器节点。
type Server struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"` // vmess|vless|trojan|shadowsocks
	Address  string `json:"address"`
	Port     int    `json:"port"`
	UUID     string `json:"uuid"` // vmess/vless 的 id;trojan 的 password
	AlterID  int    `json:"alterId"`
	Security string `json:"security"` // vmess 加密: auto|none|aes-128-gcm|chacha20-poly1305
	// Encryption 是 vless 的加密方式,通常为 "none"。
	Encryption string `json:"encryption"`
	// Flow 是 vless/trojan 的流控,例如 xtls-rprx-vision。
	Flow string `json:"flow"`
	// Network 是传输协议: tcp|ws|grpc|h2|kcp|quic。
	Network string `json:"network"`
	// TLS 是传输安全: none|tls|reality。
	TLS         string   `json:"tls"`
	Host        string   `json:"host"`
	Path        string   `json:"path"`
	SNI         string   `json:"sni"`
	ALPN        []string `json:"alpn"`
	Fingerprint string   `json:"fingerprint"`
	PublicKey   string   `json:"publicKey"` // reality pbk
	ShortID     string   `json:"shortId"`   // reality sid
	Method      string   `json:"method"`    // shadowsocks 加密
	Password    string   `json:"password"`  // shadowsocks 密码
	LatencyMS   int      `json:"latencyMs"` // -1 表示未测
	LatencyErr  string   `json:"latencyErr"`
}

// Settings 保存全局设置。
type Settings struct {
	SocksPort      int  `json:"socksPort"`
	HTTPPort       int  `json:"httpPort"`
	SetSystemProxy bool `json:"setSystemProxy"`
	AutoConnect    bool `json:"autoConnect"`
	// RouteMode 是路由模式: global|rules|direct,空值视为 rules。
	RouteMode string `json:"routeMode"`
	// CloseAction 是关闭窗口时的行为:""(未选择,首次关闭时询问)|"tray"(最小化到托盘)|"quit"(退出应用)。
	CloseAction string `json:"closeAction"`

	// 高级设置(全部映射到 v4 配置)。
	// LogLevel 是日志级别: debug|info|warning|error|none,默认 "warning"。
	LogLevel string `json:"logLevel"`
	// DomainStrategy 是路由域名策略: AsIs|IPIfNonMatch|IPOnDemand,默认 "IPIfNonMatch"。
	DomainStrategy string `json:"domainStrategy"`
	// ListenAddress 是入站监听地址,默认 "127.0.0.1"。
	ListenAddress string `json:"listenAddress"`
	// SocksUDP 是否允许 SOCKS 入站转发 UDP,默认 true。
	SocksUDP bool `json:"socksUDP"`
	// SocksUser/SocksPass 非空时 SOCKS 入站启用密码认证(auth=password)。
	SocksUser string `json:"socksUser"`
	SocksPass string `json:"socksPass"`
	// Sniffing 是否对入站流量做探测,默认 true。
	Sniffing bool `json:"sniffing"`
	// SniffingOverride 是探测覆盖协议,默认 "http,tls";可选 "http,tls,quic"。
	SniffingOverride string `json:"sniffingOverride"`
	// Mux 是否对代理出站启用多路复用。
	Mux bool `json:"mux"`
	// MuxConcurrency 是 Mux 并发数,默认 8。
	MuxConcurrency int `json:"muxConcurrency"`
	// TCPFastOpen 是否对代理出站启用 TCP Fast Open。
	TCPFastOpen bool `json:"tcpFastOpen"`
	// TCPKeepAlive 是 TCP KeepAlive 间隔秒数,0 表示不设置。
	TCPKeepAlive int `json:"tcpKeepAlive"`
	// Fwmark 是出站流量标记,0 表示不设置。
	Fwmark int `json:"fwmark"`
	// ForceDNS 是否强制 DNS(路由 53/853 走代理)。
	ForceDNS bool `json:"forceDNS"`
	// DNSServers 是逗号分隔的 DNS 服务器列表(支持 DoH 域名)。
	DNSServers string `json:"dnsServers"`
	// ProxyIgnore 是系统代理忽略列表(逗号分隔),默认 "localhost,127.0.0.0/8,::1"。
	ProxyIgnore string `json:"proxyIgnore"`
	// LatencyTimeout 是测速超时秒数,默认 3。
	LatencyTimeout int `json:"latencyTimeout"`
	// TunEnable 是否启用 TUN 网卡级代理(需 root / CAP_NET_ADMIN)。
	TunEnable bool `json:"tunEnable"`
	// TunSubnet 是 TUN 网卡地址段,默认 "10.0.0.1/24"。
	TunSubnet string `json:"tunSubnet"`
	// TunMTU 是 TUN 网卡 MTU,默认 1500。
	TunMTU int `json:"tunMTU"`
	// TunFD 是常驻 root 助手创建的 TUN 设备 fd(preopened_fd),瞬态字段:
	// 仅本次连接传给核心,不持久化(json:"-")。
	TunFD int `json:"-"`
}

// Config 是持久化到磁盘的整个 GUI 状态。
type Config struct {
	Servers      []*Server `json:"servers"`
	Settings     Settings  `json:"settings"`
	LastServerID string    `json:"lastServerID"`
}

// DefaultSettings 返回默认设置:SOCKS 10808、HTTP 10809、默认开启系统代理、
// 默认路由模式为 rules(私有网段直连),并填充高级设置默认值。
func DefaultSettings() Settings {
	return Settings{
		SocksPort:        10808,
		HTTPPort:         10809,
		SetSystemProxy:   true,
		RouteMode:        "rules",
		LogLevel:         "warning",
		DomainStrategy:   "IPIfNonMatch",
		ListenAddress:    "127.0.0.1",
		SocksUDP:         true,
		Sniffing:         true,
		SniffingOverride: "http,tls",
		MuxConcurrency:   8,
		ProxyIgnore:      "localhost,127.0.0.0/8,::1",
		LatencyTimeout:   3,
		TunSubnet:        "10.0.0.1/24",
		TunMTU:           1500,
	}
}

// ConfigDir 返回配置目录 $XDG_CONFIG_HOME/v2ray-gui 或 ~/.config/v2ray-gui。
func ConfigDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "v2ray-gui")
	}
	return filepath.Join(dir, "v2ray-gui")
}

func configPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

// LoadConfig 从用户配置目录加载配置;文件不存在时返回空配置(默认设置),不报错。
// 加载时所有服务器的延迟重置为未测状态。
func LoadConfig() (*Config, error) {
	cfg := &Config{Settings: DefaultSettings()}
	data, err := os.ReadFile(configPath())
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return cfg, err
	}
	if cfg.Settings.SocksPort == 0 {
		def := DefaultSettings()
		cfg.Settings.SocksPort = def.SocksPort
		cfg.Settings.HTTPPort = def.HTTPPort
	}
	for _, s := range cfg.Servers {
		s.LatencyMS = -1
		s.LatencyErr = ""
	}
	return cfg, nil
}

// Save 将配置写入用户配置目录,目录不存在时自动创建。
func (c *Config) Save() error {
	if err := os.MkdirAll(ConfigDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0644)
}

// FindServer 按 ID 查找服务器,未找到返回 nil。
func (c *Config) FindServer(id string) *Server {
	for _, s := range c.Servers {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// NewID 生成一个 UUID v4(手写实现,不依赖第三方库)。
func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 几乎不可能失败;失败时退回时间戳,保证可用。
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10xx
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// Key 返回用于去重的键: protocol|address|port|uuid。
func (s *Server) Key() string {
	return s.Protocol + "|" + s.Address + "|" + strconv.Itoa(s.Port) + "|" + s.UUID
}
