package app

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"v2ray-gui/internal/core"
)

// ManualServerDialog 显示添加/编辑服务器表单。
// existing 为 nil 时新增(自动去重),否则编辑替换原节点(ID 保持不变)。
// onSaved 在保存成功后回调(用于刷新表格)。
func ManualServerDialog(parent fyne.Window, cfg *core.Config, existing *core.Server, onSaved func(*core.Server)) {
	f := newServerForm(existing)
	scroll := container.NewScroll(f.form())
	// TextWrapOff 后长文本不再折行,限制外层滚动容器最小尺寸避免把对话框撑宽。
	scroll.SetMinSize(fyne.NewSize(560, 480))
	// 输入框把滚轮事件转发给外层滚动容器,悬停在输入框上时滚动正常生效。
	for _, e := range f.scrollEntries {
		e.SetParent(scroll)
	}
	var refresh func()
	refresh = func() {
		scroll.Content = f.form()
		scroll.Refresh()
	}
	// 协议/TLS/传输协议变化时重建表单(字段随协议变化)。
	f.protocol.OnChanged = func(string) { refresh() }
	f.tls.OnChanged = func(string) { refresh() }
	f.network.OnChanged = func(string) { refresh() }

	title := "添加服务器"
	if existing != nil {
		title = "编辑服务器"
	}
	d := dialog.NewCustomConfirm(title, "保存", "取消", container.NewPadded(scroll), func(ok bool) {
		if !ok {
			return
		}
		srv, err := f.collect()
		if err != nil {
			dialog.ShowError(err, parent)
			return
		}
		applyServerToConfig(cfg, existing, srv)
		if err := cfg.Save(); err != nil {
			dialog.ShowError(err, parent)
			return
		}
		if onSaved != nil {
			onSaved(srv)
		}
	}, parent)
	d.Resize(fyne.NewSize(640, 520))
	d.Show()
}

// serverForm 封装服务器表单的全部控件。
// 输入框统一用 ScrollEntry:去掉内部滚动容器,滚轮事件转发给外层滚动容器。
type serverForm struct {
	name       *ScrollEntry
	protocol   *widget.Select
	address    *ScrollEntry
	port       *ScrollEntry
	uuid       *ScrollEntry // 标签随协议: ID / UUID / 密码
	alterID    *ScrollEntry
	vmessSec   *widget.Select
	flow       *ScrollEntry
	ssMethod   *widget.Select
	ssPassword *ScrollEntry
	network    *widget.Select
	tls        *widget.Select
	tlsWarn    *widget.Label
	host       *ScrollEntry
	path       *ScrollEntry
	sni        *ScrollEntry
	alpn       *ScrollEntry
	fp         *widget.Select
	pbk        *ScrollEntry
	sid        *ScrollEntry

	// scrollEntries 收集全部输入框,外层滚动容器创建后统一 SetParent。
	scrollEntries []*ScrollEntry
}

func newServerForm(existing *core.Server) *serverForm {
	f := &serverForm{}
	// entry 创建输入框并登记到 scrollEntries,便于后续绑定外层滚动容器。
	entry := func() *ScrollEntry {
		e := NewScrollEntry()
		f.scrollEntries = append(f.scrollEntries, e)
		return e
	}
	f.name = entry()
	f.protocol = widget.NewSelect([]string{"vmess", "vless", "trojan", "shadowsocks"}, nil)
	f.address = entry()
	f.port = entry()
	f.uuid = entry()
	f.alterID = entry()
	f.vmessSec = widget.NewSelect([]string{"auto", "none", "aes-128-gcm", "chacha20-poly1305"}, nil)
	f.flow = entry()
	f.ssMethod = widget.NewSelect([]string{"aes-256-gcm", "aes-128-gcm", "chacha20-poly1305", "chacha20-ietf-poly1305", "none"}, nil)
	f.ssPassword = entry()
	f.network = widget.NewSelect([]string{"tcp", "ws", "grpc", "h2", "kcp", "quic"}, nil)
	f.tls = widget.NewSelect([]string{"none", "tls", "reality"}, nil)
	f.host = entry()
	f.path = entry()
	f.sni = entry()
	f.alpn = entry()
	f.fp = widget.NewSelect([]string{"", "chrome", "firefox", "safari", "random"}, nil)
	f.pbk = entry()
	f.sid = entry()
	f.tlsWarn = widget.NewLabel("需要 Xray-core,本核心不支持")
	f.tlsWarn.Importance = widget.DangerImportance
	f.port.SetPlaceHolder("1-65535")
	f.alterID.SetText("0")
	if existing != nil {
		f.prefill(existing)
	}
	return f
}

func (f *serverForm) prefill(s *core.Server) {
	f.name.SetText(s.Name)
	f.address.SetText(s.Address)
	f.port.SetText(strconv.Itoa(s.Port))
	f.uuid.SetText(s.UUID)
	f.alterID.SetText(strconv.Itoa(s.AlterID))
	f.flow.SetText(s.Flow)
	f.host.SetText(s.Host)
	f.path.SetText(s.Path)
	f.sni.SetText(s.SNI)
	f.alpn.SetText(strings.Join(s.ALPN, ","))
	f.pbk.SetText(s.PublicKey)
	f.sid.SetText(s.ShortID)
	if s.Security != "" {
		f.vmessSec.SetSelected(s.Security)
	}
	if s.Method != "" {
		f.ssMethod.SetSelected(s.Method)
	}
	if s.Network != "" {
		f.network.SetSelected(s.Network)
	}
	if s.TLS != "" {
		f.tls.SetSelected(s.TLS)
	}
	if s.Fingerprint != "" {
		f.fp.SetSelected(s.Fingerprint)
	}
	if s.Protocol != "" {
		f.protocol.SetSelected(s.Protocol)
	}
}

// form 按当前协议/TLS 状态构建表单,分"基础信息"与"传输与安全"两组。
func (f *serverForm) form() *widget.Form {
	form := widget.NewForm(
		widget.NewFormItem("名称", f.name),
		widget.NewFormItem("协议", f.protocol),
		widget.NewFormItem("地址", f.address),
		widget.NewFormItem("端口", f.port),
	)
	// 基础信息:协议相关的身份/凭据字段。
	switch f.protocol.Selected {
	case "vmess":
		form.Append("ID", f.uuid)
		form.Append("alterId", f.alterID)
		form.Append("vmess 加密", f.vmessSec)
	case "vless":
		form.Append("UUID", f.uuid)
	case "trojan":
		form.Append("密码", f.uuid)
	case "shadowsocks":
		form.Append("加密方式", f.ssMethod)
		form.Append("密码", f.ssPassword)
	}
	// 分组:基础信息与传输/安全之间放分隔线 + 灰色小标题。
	form.Append("", widget.NewSeparator())
	hint := widget.NewLabel("传输协议与 TLS 设置")
	hint.Importance = widget.LowImportance
	form.Append("", hint)
	// 传输与安全。
	form.Append("传输协议", f.network)
	form.Append("TLS", f.tls)
	if f.tls.Selected == "reality" {
		form.Append("", f.tlsWarn)
	}
	form.Append("Host", f.host)
	pathLabel := "Path"
	if f.network.Selected == "grpc" {
		pathLabel = "ServiceName"
	}
	form.Append(pathLabel, f.path)
	form.Append("SNI", f.sni)
	form.Append("ALPN (逗号分隔)", f.alpn)
	form.Append("指纹", f.fp)
	form.Append("reality 公钥", f.pbk)
	form.Append("reality 短ID", f.sid)
	if f.protocol.Selected == "vless" || f.protocol.Selected == "trojan" {
		form.Append("flow", f.flow)
	}
	return form
}

// collect 校验并收集表单内容为 Server。
func (f *serverForm) collect() (*core.Server, error) {
	addr := strings.TrimSpace(f.address.Text)
	if addr == "" {
		return nil, errors.New("地址不能为空")
	}
	port, err := strconv.Atoi(strings.TrimSpace(f.port.Text))
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("端口必须是 1-65535 的整数")
	}
	s := &core.Server{
		ID:          core.NewID(),
		Name:        strings.TrimSpace(f.name.Text),
		Protocol:    f.protocol.Selected,
		Address:     addr,
		Port:        port,
		Network:     f.network.Selected,
		TLS:         f.tls.Selected,
		Host:        strings.TrimSpace(f.host.Text),
		Path:        strings.TrimSpace(f.path.Text),
		SNI:         strings.TrimSpace(f.sni.Text),
		ALPN:        splitAlpn(f.alpn.Text),
		Fingerprint: f.fp.Selected,
		PublicKey:   strings.TrimSpace(f.pbk.Text),
		ShortID:     strings.TrimSpace(f.sid.Text),
		LatencyMS:   -1,
	}
	switch f.protocol.Selected {
	case "vmess":
		if strings.TrimSpace(f.uuid.Text) == "" {
			return nil, errors.New("vmess 需要填写 ID")
		}
		s.UUID = strings.TrimSpace(f.uuid.Text)
		s.AlterID, _ = strconv.Atoi(strings.TrimSpace(f.alterID.Text))
		s.Security = f.vmessSec.Selected
	case "vless":
		if strings.TrimSpace(f.uuid.Text) == "" {
			return nil, errors.New("vless 需要填写 UUID")
		}
		s.UUID = strings.TrimSpace(f.uuid.Text)
		s.Encryption = "none"
		s.Flow = strings.TrimSpace(f.flow.Text)
	case "trojan":
		if strings.TrimSpace(f.uuid.Text) == "" {
			return nil, errors.New("trojan 需要填写密码")
		}
		s.UUID = strings.TrimSpace(f.uuid.Text)
		s.Flow = strings.TrimSpace(f.flow.Text)
	case "shadowsocks":
		if strings.TrimSpace(f.ssPassword.Text) == "" {
			return nil, errors.New("shadowsocks 需要填写密码")
		}
		s.Method = f.ssMethod.Selected
		s.Password = strings.TrimSpace(f.ssPassword.Text)
	default:
		return nil, errors.New("请选择协议")
	}
	if s.Name == "" {
		s.Name = net.JoinHostPort(s.Address, strconv.Itoa(s.Port))
	}
	return s, nil
}

// applyServerToConfig 将收集到的节点写入配置:编辑时替换原节点(保留原 ID),
// 新增时按 Key 去重。
func applyServerToConfig(cfg *core.Config, existing, srv *core.Server) {
	if existing != nil {
		for i, s := range cfg.Servers {
			if s.ID == existing.ID {
				srv.ID = existing.ID // 保留原 ID,避免 LastServerID 失效
				cfg.Servers[i] = srv
				return
			}
		}
		// existing 已被删除,当作新增处理。
		cfg.Servers = append(cfg.Servers, srv)
		return
	}
	for _, s := range cfg.Servers {
		if s.Key() == srv.Key() {
			return // 已存在,跳过(去重)
		}
	}
	cfg.Servers = append(cfg.Servers, srv)
}

// SettingsDialog 显示设置对话框:基础设置(端口/系统代理/自动连接)+
// 高级设置(20 项,映射到 v4 配置)。保存成功后写回 cfg.Settings 并持久化。
func SettingsDialog(parent fyne.Window, cfg *core.Config, onSaved func()) {
	st := cfg.Settings

	// scrollEntry 创建可转发滚轮事件的输入框(高级设置页签在滚动容器里,
	// 普通 Entry 的内部滚动容器会吞掉滚轮事件),并登记到 scrollEntries,
	// 外层滚动容器创建后统一 SetParent(与 ManualServerDialog 一致)。
	var scrollEntries []*ScrollEntry
	scrollEntry := func() *ScrollEntry {
		e := NewScrollEntry()
		scrollEntries = append(scrollEntries, e)
		return e
	}

	// ---- 基础设置 ----
	socks := widget.NewEntry()
	socks.SetText(strconv.Itoa(st.SocksPort))
	socks.Validator = portValidator(1024, 65535, "SOCKS 端口")

	http := widget.NewEntry()
	http.SetText(strconv.Itoa(st.HTTPPort))
	http.Validator = portValidator(1024, 65535, "HTTP 端口")

	sysProxy := widget.NewCheck("连接时自动设置系统代理", nil)
	sysProxy.SetChecked(st.SetSystemProxy)

	auto := widget.NewCheck("启动时自动连接上次节点", nil)
	auto.SetChecked(st.AutoConnect)

	basic := widget.NewForm(
		widget.NewFormItem("SOCKS 端口 (1024-65535)", socks),
		widget.NewFormItem("HTTP 端口 (1024-65535)", http),
		widget.NewFormItem("系统代理", sysProxy),
		widget.NewFormItem("自动连接", auto),
	)

	// ---- 高级设置(20 项)----
	logLevel := widget.NewSelect([]string{"debug", "info", "warning", "error", "none"}, nil)
	selectDefault(logLevel, st.LogLevel, "warning")

	domainStrategy := widget.NewSelect([]string{"AsIs", "IPIfNonMatch", "IPOnDemand"}, nil)
	selectDefault(domainStrategy, st.DomainStrategy, "IPIfNonMatch")

	listen := scrollEntry()
	listen.SetText(defaultStr(st.ListenAddress, "127.0.0.1"))

	socksUDP := widget.NewCheck("", nil)
	socksUDP.SetChecked(st.SocksUDP)

	socksUser := scrollEntry()
	socksUser.SetText(st.SocksUser)

	socksPass := scrollEntry()
	socksPass.Password = true // 密码模式:隐藏明文,渲染时自动加密码可见性切换按钮
	socksPass.SetText(st.SocksPass)

	sniffing := widget.NewCheck("", nil)
	sniffing.SetChecked(st.Sniffing)

	sniffOverride := widget.NewSelect([]string{"http,tls", "http,tls,quic"}, nil)
	selectDefault(sniffOverride, st.SniffingOverride, "http,tls")

	mux := widget.NewCheck("", nil)
	mux.SetChecked(st.Mux)

	muxConcurrency := scrollEntry()
	muxConcurrency.SetText(strconv.Itoa(defaultInt(st.MuxConcurrency, 8)))

	tcpFastOpen := widget.NewCheck("", nil)
	tcpFastOpen.SetChecked(st.TCPFastOpen)

	tcpKeepAlive := scrollEntry()
	tcpKeepAlive.SetText(strconv.Itoa(st.TCPKeepAlive))

	fwmark := scrollEntry()
	fwmark.SetText(strconv.Itoa(st.Fwmark))

	forceDNS := widget.NewCheck("", nil)
	forceDNS.SetChecked(st.ForceDNS)

	dnsServers := scrollEntry()
	dnsServers.SetText(st.DNSServers)
	dnsServers.SetPlaceHolder("https://dns.google/dns-query,1.1.1.1")

	proxyIgnore := scrollEntry()
	proxyIgnore.SetText(defaultStr(st.ProxyIgnore, "localhost,127.0.0.0/8,::1"))

	latencyTimeout := scrollEntry()
	latencyTimeout.SetText(strconv.Itoa(defaultInt(st.LatencyTimeout, 3)))

	tunEnable := widget.NewCheck("", nil)
	tunEnable.SetChecked(st.TunEnable)

	tunSubnet := scrollEntry()
	tunSubnet.SetText(defaultStr(st.TunSubnet, "10.0.0.1/24"))
	tunSubnet.SetPlaceHolder("10.0.0.1/24")

	tunMTU := scrollEntry()
	tunMTU.SetText(strconv.Itoa(defaultInt(st.TunMTU, 1500)))

	tunWarn := widget.NewLabel("TUN 需要 root 权限运行,并会修改系统路由表,断开时自动还原")
	tunWarn.Importance = widget.DangerImportance
	if st.TunEnable {
		tunWarn.Show()
	} else {
		tunWarn.Hide()
	}
	tunEnable.OnChanged = func(on bool) {
		if on {
			tunWarn.Show()
		} else {
			tunWarn.Hide()
		}
	}

	advForm := widget.NewForm(
		widget.NewFormItem("日志级别", logLevel),
		widget.NewFormItem("域名策略", domainStrategy),
		widget.NewFormItem("入站监听地址", listen),
		widget.NewFormItem("SOCKS UDP 转发", socksUDP),
		widget.NewFormItem("SOCKS 认证用户名", socksUser),
		widget.NewFormItem("SOCKS 认证密码", socksPass),
		widget.NewFormItem("入站流量探测", sniffing),
		widget.NewFormItem("探测覆盖", sniffOverride),
		widget.NewFormItem("Mux 多路复用", mux),
		widget.NewFormItem("Mux 并发数", muxConcurrency),
		widget.NewFormItem("TCP Fast Open", tcpFastOpen),
		widget.NewFormItem("TCP KeepAlive 间隔(秒)", tcpKeepAlive),
		widget.NewFormItem("出站 fwmark", fwmark),
		widget.NewFormItem("强制 DNS(路由 53/853 走代理)", forceDNS),
		widget.NewFormItem("DNS 服务器", dnsServers),
		widget.NewFormItem("系统代理忽略列表", proxyIgnore),
		widget.NewFormItem("测速超时(秒)", latencyTimeout),
		widget.NewFormItem("TUN 网卡代理(需 root)", tunEnable),
		widget.NewFormItem("TUN 子网", tunSubnet),
		widget.NewFormItem("TUN MTU", tunMTU),
	)
	advScroll := container.NewScroll(container.NewVBox(advForm, tunWarn))
	advScroll.SetMinSize(fyne.NewSize(560, 420))
	// 输入框把滚轮事件转发给外层滚动容器,悬停在输入框上时滚动正常生效。
	for _, e := range scrollEntries {
		e.SetParent(advScroll)
	}

	tabs := container.NewAppTabs(
		container.NewTabItem("基础设置", basic),
		container.NewTabItem("高级设置", advScroll),
	)

	d := dialog.NewCustomConfirm("设置", "保存", "取消", tabs, func(ok bool) {
		if !ok {
			return
		}
		newSt, err := collectSettings(st, collectArgs{
			socks: socks.Text, http: http.Text,
			sysProxy: sysProxy.Checked, auto: auto.Checked,
			logLevel: logLevel.Selected, domainStrategy: domainStrategy.Selected,
			listen:   listen.Text,
			socksUDP: socksUDP.Checked, socksUser: socksUser.Text, socksPass: socksPass.Text,
			sniffing: sniffing.Checked, sniffOverride: sniffOverride.Selected,
			mux: mux.Checked, muxConcurrency: muxConcurrency.Text,
			tcpFastOpen: tcpFastOpen.Checked, tcpKeepAlive: tcpKeepAlive.Text,
			fwmark:   fwmark.Text,
			forceDNS: forceDNS.Checked, dnsServers: dnsServers.Text,
			proxyIgnore: proxyIgnore.Text, latencyTimeout: latencyTimeout.Text,
			tunEnable: tunEnable.Checked, tunSubnet: tunSubnet.Text, tunMTU: tunMTU.Text,
		})
		if err != nil {
			dialog.ShowError(err, parent)
			return
		}
		cfg.Settings = newSt
		if err := cfg.Save(); err != nil {
			dialog.ShowError(err, parent)
			return
		}
		if onSaved != nil {
			onSaved()
		}
	}, parent)
	d.Resize(fyne.NewSize(700, 560))
	d.Show()
}

// collectArgs 是 SettingsDialog 收集的全部表单值。
type collectArgs struct {
	socks, http                      string
	sysProxy, auto                   bool
	logLevel, domainStrategy, listen string
	socksUDP                         bool
	socksUser, socksPass             string
	sniffing                         bool
	sniffOverride                    string
	mux                              bool
	muxConcurrency                   string
	tcpFastOpen                      bool
	tcpKeepAlive, fwmark             string
	forceDNS                         bool
	dnsServers, proxyIgnore          string
	latencyTimeout                   string
	tunEnable                        bool
	tunSubnet, tunMTU                string
}

// collectSettings 校验设置表单并写回 Settings;任一校验失败返回错误且不修改。
func collectSettings(base core.Settings, a collectArgs) (core.Settings, error) {
	if strings.TrimSpace(a.listen) == "" {
		return base, errors.New("监听地址不能为空")
	}
	p1, err := strconv.Atoi(strings.TrimSpace(a.socks))
	if err != nil || p1 < 1024 || p1 > 65535 {
		return base, errors.New("SOCKS 端口必须是 1024-65535 的整数")
	}
	p2, err := strconv.Atoi(strings.TrimSpace(a.http))
	if err != nil || p2 < 1024 || p2 > 65535 {
		return base, errors.New("HTTP 端口必须是 1024-65535 的整数")
	}
	muxN, err := strconv.Atoi(strings.TrimSpace(a.muxConcurrency))
	if err != nil || muxN < 1 || muxN > 1024 {
		return base, errors.New("Mux 并发数必须是 1-1024 的整数")
	}
	keepAlive, err := strconv.Atoi(strings.TrimSpace(a.tcpKeepAlive))
	if err != nil || keepAlive < 0 || keepAlive > 3600 {
		return base, errors.New("TCP KeepAlive 必须是 0-3600 的整数")
	}
	fmark, err := strconv.Atoi(strings.TrimSpace(a.fwmark))
	if err != nil || fmark < 0 || fmark > 4294967295 {
		return base, errors.New("fwmark 必须是 0-4294967295 的整数")
	}
	timeout, err := strconv.Atoi(strings.TrimSpace(a.latencyTimeout))
	if err != nil || timeout < 1 || timeout > 60 {
		return base, errors.New("测速超时必须是 1-60 的整数")
	}
	mtu, err := strconv.Atoi(strings.TrimSpace(a.tunMTU))
	if err != nil || mtu < 576 || mtu > 65535 {
		return base, errors.New("TUN MTU 必须是 576-65535 的整数")
	}
	subnet := strings.TrimSpace(a.tunSubnet)
	if _, _, err := net.ParseCIDR(subnet); err != nil {
		return base, errors.New("TUN 子网必须是合法网段,如 10.0.0.1/24")
	}
	base.SocksPort = p1
	base.HTTPPort = p2
	base.SetSystemProxy = a.sysProxy
	base.AutoConnect = a.auto
	base.LogLevel = a.logLevel
	base.DomainStrategy = a.domainStrategy
	base.ListenAddress = strings.TrimSpace(a.listen)
	base.SocksUDP = a.socksUDP
	base.SocksUser = strings.TrimSpace(a.socksUser)
	base.SocksPass = a.socksPass
	base.Sniffing = a.sniffing
	base.SniffingOverride = a.sniffOverride
	base.Mux = a.mux
	base.MuxConcurrency = muxN
	base.TCPFastOpen = a.tcpFastOpen
	base.TCPKeepAlive = keepAlive
	base.Fwmark = fmark
	base.ForceDNS = a.forceDNS
	base.DNSServers = strings.TrimSpace(a.dnsServers)
	base.ProxyIgnore = strings.TrimSpace(a.proxyIgnore)
	base.LatencyTimeout = timeout
	base.TunEnable = a.tunEnable
	base.TunSubnet = subnet
	base.TunMTU = mtu
	return base, nil
}

// selectDefault 仅在候选值合法时选中;否则回退到默认值。
func selectDefault(sel *widget.Select, value, def string) {
	for _, opt := range sel.Options {
		if opt == value {
			sel.SetSelected(value)
			return
		}
	}
	sel.SetSelected(def)
}

// defaultStr 空值时返回默认值。
func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// defaultInt 零值时返回默认值。
func defaultInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// ImportURLDialog 显示输入订阅地址的对话框,确认后回调 URL。
func ImportURLDialog(parent fyne.Window, onDone func(url string)) {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("https://example.com/subscribe")
	items := []*widget.FormItem{widget.NewFormItem("订阅地址", entry)}
	d := dialog.NewForm("导入订阅", "导入", "取消", items, func(ok bool) {
		if ok && onDone != nil {
			onDone(strings.TrimSpace(entry.Text))
		}
	}, parent)
	d.Resize(fyne.NewSize(620, 220))
	d.Show()
}

// ConfirmDialog 显示确认对话框,点确定时回调 onOK。
func ConfirmDialog(parent fyne.Window, title, msg string, onOK func()) {
	d := dialog.NewConfirm(title, msg, func(ok bool) {
		if ok && onOK != nil {
			onOK()
		}
	}, parent)
	d.SetConfirmText("确定")
	d.Show()
}

// portValidator 校验端口输入。
func portValidator(min, max int, name string) fyne.StringValidator {
	return func(s string) error {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < min || n > max {
			return errors.New(name + "必须是 " + strconv.Itoa(min) + "-" + strconv.Itoa(max) + " 的整数")
		}
		return nil
	}
}

// splitAlpn 将逗号分隔的 ALPN 文本转为切片。
func splitAlpn(s string) []string {
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
