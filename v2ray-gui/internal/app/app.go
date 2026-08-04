// Package app 实现主窗口、布局与事件处理。
package app

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"v2ray-gui/internal/core"
	"v2ray-gui/internal/engine"
)

var tableHeaders = []string{"名称", "协议", "地址", "端口", "TLS", "延迟"}

// guiApp 是主窗口的界面状态。
type guiApp struct {
	fyneApp fyne.App
	win     fyne.Window
	cfg     *core.Config
	logView *LogView
	table   *widget.Table
	// selected 是当前选中的数据行下标,-1 表示无选中。
	selected int

	engine       *engine.Engine
	errAtStartup error
	statusCircle *canvas.Circle // 左侧连接状态圆点
	statusTxt    *widget.Label  // 左侧连接状态文本
	proxyTxt     *widget.Label  // 系统代理状态
	speedTxt     *widget.Label  // 流量累计 + 实时速度
	uptimeTxt    *widget.Label  // 运行时长
	proxyOn      bool
	proxyFail    bool
	// tunActive 表示 TUN 网卡路由是否已配置(断开时需还原)。
	tunActive  bool
	lastUp     int64 // 上一秒累计上行,用于计算实时速率
	lastDown   int64 // 上一秒累计下行,用于计算实时速率
	connectBtn *widget.Button
	speedBtn   *widget.Button
	// currentSrv 是当前已连接的服务器,断开后置 nil;切换路由模式时用它重连。
	currentSrv *core.Server
	testing    bool
	ticker     *time.Ticker
	stopTicker chan struct{}
	lastStatus string // 上次状态栏内容指纹,updateStatus 变化检测用
}

// Run 启动 GUI 主循环。
func Run() {
	g := &guiApp{selected: -1}
	g.fyneApp = app.NewWithID("com.v2ray.gui")
	// 应用图标:启动器/任务栏与窗口共用(assets/icon.png,见 icon.go)。
	g.fyneApp.SetIcon(appIcon)
	// 内置 Noto Sans SC 字体;SetTheme 之后不会被磁盘设置覆盖(settings.go 的
	// themeSpecified 保护,即使后续设置文件变化也保持自定义主题)。
	g.fyneApp.Settings().SetTheme(notoTheme{})
	g.win = g.fyneApp.NewWindow("v2rayG")
	g.win.SetIcon(appIcon)
	g.win.Resize(fyne.NewSize(980, 640))
	g.win.CenterOnScreen()

	cfg, err := core.LoadConfig()
	if err != nil {
		cfg = &core.Config{Settings: core.DefaultSettings()}
		g.errAtStartup = err
	}
	g.cfg = cfg
	g.logView = NewLogView()
	g.engine = engine.NewEngine(func(line string) {
		g.logView.Append(line)
	})

	// 启动时自动提权:TUN 开启且非 root 时,通过 pkexec 以 root 重启自身。
	// 提权后的实例带 V2RAY_GUI_ELEVATED=1,不会再触发本逻辑;root 实例也不走。
	// 提权失败时继续以普通权限运行,连接 TUN 时仍会二次提示。
	var elevateErr error
	if cfg.Settings.TunEnable && core.NeedsRoot() && os.Getenv("V2RAY_GUI_ELEVATED") != "1" {
		if err := relaunchElevated(cfg.LastServerID); err != nil {
			elevateErr = err
		} else {
			g.logView.Append("正在通过 pkexec 请求管理员权限...")
			g.fyneApp.Quit()
			return
		}
	}

	g.buildUI()
	g.win.SetCloseIntercept(g.onClose)
	g.win.Show()
	if g.errAtStartup != nil {
		dialog.ShowError(fmt.Errorf("读取配置失败,已使用默认配置:%v", g.errAtStartup), g.win)
	}
	if elevateErr != nil {
		dialog.ShowError(fmt.Errorf("无法通过 pkexec 提权:%v。TUN 需要 root,可手动用 pkexec ./v2ray-gui 启动", elevateErr), g.win)
	}

	// 启动时自动连接上次节点。
	if g.cfg.Settings.AutoConnect && g.cfg.LastServerID != "" {
		if srv := g.cfg.FindServer(g.cfg.LastServerID); srv != nil {
			time.AfterFunc(500*time.Millisecond, func() {
				fyne.Do(func() { g.connectTo(srv) })
			})
		}
	}
	// 提权实例自动连接:pkexec 重启后由 V2RAY_GUI_CONNECT 指定要连接的节点。
	if os.Getenv("V2RAY_GUI_ELEVATED") == "1" {
		if id := os.Getenv("V2RAY_GUI_CONNECT"); id != "" {
			if srv := g.cfg.FindServer(id); srv != nil {
				time.AfterFunc(500*time.Millisecond, func() {
					fyne.Do(func() { g.connectTo(srv) })
				})
			}
		}
	}
	g.fyneApp.Run()
}

// buildUI 组装工具栏、表格、日志页签与状态栏。
func (g *guiApp) buildUI() {
	g.buildTable()
	toolbar := g.buildToolbar()
	status := g.buildStatus()
	tabs := container.NewAppTabs(
		container.NewTabItem("服务器", widget.NewCard("节点列表", "", g.table)),
		container.NewTabItem("日志", widget.NewCard("运行日志", "", g.logView.Content())),
	)
	// 底部:内容与状态栏之间加分隔线。
	bottomBar := container.NewVBox(widget.NewSeparator(), status)
	g.win.SetContent(container.NewBorder(toolbar, bottomBar, nil, nil, tabs))

	// 每秒刷新状态栏(运行时长与流量)。
	g.ticker = time.NewTicker(time.Second)
	g.stopTicker = make(chan struct{})
	go func() {
		for {
			select {
			case <-g.ticker.C:
				fyne.Do(g.updateStatus)
			case <-g.stopTicker:
				return
			}
		}
	}()
	g.updateStatus()
}

// buildToolbar 构建顶部按钮栏:按功能分组,组间用分隔线,整体加内边距。
func (g *guiApp) buildToolbar() fyne.CanvasObject {
	importClip := widget.NewButtonWithIcon("导入剪贴板", theme.ContentPasteIcon(), g.importClipboard)
	importSub := widget.NewButtonWithIcon("导入订阅", theme.DownloadIcon(), g.importSubscribe)
	addBtn := widget.NewButtonWithIcon("添加", theme.ContentAddIcon(), g.addServer)
	editBtn := widget.NewButtonWithIcon("编辑", theme.DocumentCreateIcon(), g.editServer)
	delBtn := widget.NewButtonWithIcon("删除", theme.DeleteIcon(), g.deleteServer)
	g.speedBtn = widget.NewButtonWithIcon("测速", theme.MediaFastForwardIcon(), g.speedTest)
	g.connectBtn = widget.NewButtonWithIcon("连接", theme.MediaPlayIcon(), g.toggleConnect)
	setBtn := widget.NewButtonWithIcon("设置", theme.SettingsIcon(), g.settingsDialog)
	aboutBtn := widget.NewButtonWithIcon("关于", theme.HelpIcon(), g.aboutDialog)
	// 连接按钮常显主色,更突出。
	g.connectBtn.Importance = widget.HighImportance
	// 路由模式下拉:全局 / 规则 / 直连,切换后自动重连生效。
	modeSel := widget.NewSelect([]string{"全局", "规则", "直连"}, g.onRouteModeChanged)
	modeSel.SetSelected(routeModeLabel(g.cfg.Settings.RouteMode))

	return container.NewPadded(container.NewHBox(
		importClip, importSub,
		widget.NewSeparator(),
		addBtn, editBtn, delBtn,
		widget.NewSeparator(),
		g.speedBtn, g.connectBtn, modeSel,
		widget.NewSeparator(),
		setBtn, aboutBtn,
	))
}

// routeModeValue 将界面文案转为配置值:全局→global,规则→rules,直连→direct。
func routeModeValue(label string) string {
	switch label {
	case "全局":
		return "global"
	case "直连":
		return "direct"
	default:
		return "rules"
	}
}

// routeModeLabel 将配置值转为界面文案;""(旧配置)视为 rules。
func routeModeLabel(v string) string {
	switch v {
	case "global":
		return "全局"
	case "direct":
		return "直连"
	default:
		return "规则"
	}
}

// onRouteModeChanged 处理路由模式切换:写回配置并保存;
// 核心运行中时断开并按新模式自动重连,立即生效。
func (g *guiApp) onRouteModeChanged(label string) {
	v := routeModeValue(label)
	if v == g.cfg.Settings.RouteMode {
		return
	}
	g.cfg.Settings.RouteMode = v
	_ = g.cfg.Save()
	g.logView.Append("路由模式: " + label)
	if g.engine.Running() && g.currentSrv != nil {
		srv := g.currentSrv
		g.disconnect()
		g.connectTo(srv)
	}
}

// buildTable 构建服务器表格,第 0 行为表头。
func (g *guiApp) buildTable() {
	g.table = widget.NewTable(
		func() (int, int) {
			return len(g.cfg.Servers) + 1, len(tableHeaders)
		},
		newTableCell,
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			cell := obj.(*fyne.Container)
			bg := cell.Objects[0].(*canvas.Rectangle)
			label := cell.Objects[1].(*widget.Label)
			// 偶数数据行(第 2、4、6...行)加轻微底色做斑马纹;表头无底色。
			bg.Hide()
			if id.Row > 0 && (id.Row-1)%2 == 1 {
				bg.Show()
			}
			if id.Row == 0 {
				label.TextStyle = fyne.TextStyle{Bold: true}
				label.Importance = widget.MediumImportance
				label.SetText(tableHeaders[id.Col])
				return
			}
			label.TextStyle = fyne.TextStyle{}
			label.Importance = widget.MediumImportance
			srv := g.cfg.Servers[id.Row-1]
			switch id.Col {
			case 0:
				label.SetText(srv.Name)
			case 1:
				label.SetText(srv.Protocol)
			case 2:
				label.SetText(srv.Address)
			case 3:
				label.SetText(strconv.Itoa(srv.Port))
			case 4:
				label.SetText(srv.TLS)
			case 5:
				label.SetText(latencyText(srv))
				label.Importance = latencyImportance(srv)
			}
			label.Refresh()
			cell.Refresh()
		},
	)
	g.table.SetColumnWidth(0, 200)
	g.table.SetColumnWidth(1, 100)
	g.table.SetColumnWidth(2, 200)
	g.table.SetColumnWidth(3, 70)
	g.table.SetColumnWidth(4, 80)
	g.table.SetColumnWidth(5, 100)
	// 表头第 0 行不可选中。
	g.table.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			g.table.UnselectAll()
			return
		}
		g.selected = id.Row - 1
	}
	g.table.OnUnselected = func(id widget.TableCellID) {
		if id.Row > 0 && g.selected == id.Row-1 {
			g.selected = -1
		}
	}
}

// buildStatus 构建底部状态栏:左侧连接状态圆点 + 文本,右侧代理/速度/运行时长。
func (g *guiApp) buildStatus() fyne.CanvasObject {
	g.statusCircle = canvas.NewCircle(theme.ErrorColor())
	g.statusTxt = widget.NewLabel("未连接")
	g.statusTxt.Importance = widget.DangerImportance
	g.proxyTxt = widget.NewLabel("")
	g.proxyTxt.Importance = widget.LowImportance
	g.speedTxt = widget.NewLabel("")
	g.speedTxt.Importance = widget.LowImportance
	g.uptimeTxt = widget.NewLabel("")
	g.uptimeTxt.Importance = widget.LowImportance

	// GridWrap 固定圆点 10x10,与文本在 HBox 中顶部对齐。
	circleBox := container.NewGridWrap(fyne.NewSize(10, 10), g.statusCircle)
	left := container.NewHBox(circleBox, g.statusTxt)
	right := container.NewHBox(g.proxyTxt, g.speedTxt, g.uptimeTxt)
	return container.NewBorder(nil, nil, left, right, nil)
}

// newTableCell 创建固定行高(36)的表格单元格:斑马纹背景矩形 + 居中文本。
func newTableCell() fyne.CanvasObject {
	bg := canvas.NewRectangle(theme.HoverColor())
	bg.SetMinSize(fyne.NewSize(1, 36)) // 决定表格行高为 36
	bg.Hide()
	label := widget.NewLabel("")
	return container.NewStack(bg, label)
}

// latencyImportance 返回延迟列的文本颜色:越快越绿,失败红色,未测默认色。
func latencyImportance(s *core.Server) widget.Importance {
	switch {
	case s.LatencyMS < 0 && s.LatencyErr != "": // 测速失败
		return widget.DangerImportance
	case s.LatencyMS < 0: // 未测
		return widget.MediumImportance
	case s.LatencyMS < 200:
		return widget.SuccessImportance
	case s.LatencyMS < 500:
		return widget.WarningImportance
	default:
		return widget.DangerImportance
	}
}

// updateStatus 刷新状态栏与连接按钮。
// 先把全部新值算好并与上次渲染的内容指纹比较,一致则直接返回,
// 避免每秒 ticker 在内容不变(如未连接)时反复 SetText/Refresh 造成无谓重绘。
func (g *guiApp) updateStatus() {
	running := g.engine.Running()
	var (
		statusTxt    string
		statusImport widget.Importance
		speedTxt     string
		uptimeTxt    string
		proxyImport  widget.Importance
		circleColor  = theme.ErrorColor()
	)
	if running {
		up, down := g.engine.Traffic()
		// 每秒增量即实时速率(首次采样时 last 为 0,近似当前累计值)。
		upSpeed := up - g.lastUp
		downSpeed := down - g.lastDown
		g.lastUp, g.lastDown = up, down
		if upSpeed < 0 {
			upSpeed = 0
		}
		if downSpeed < 0 {
			downSpeed = 0
		}
		statusTxt = fmt.Sprintf("已连接 SOCKS:%d HTTP:%d", g.cfg.Settings.SocksPort, g.cfg.Settings.HTTPPort)
		statusImport = widget.SuccessImportance
		circleColor = theme.SuccessColor()
		speedTxt = fmt.Sprintf("↑ %s ↓ %s · 速度 ↑ %s/s ↓ %s/s",
			formatBytes(up), formatBytes(down), formatBytes(upSpeed), formatBytes(downSpeed))
		uptimeTxt = "运行 " + formatDuration(g.engine.Uptime())
	} else {
		statusTxt = "未连接"
		statusImport = widget.DangerImportance
		speedTxt = "↑ 0 B ↓ 0 B"
		uptimeTxt = "运行 00:00:00"
		g.lastUp, g.lastDown = 0, 0
	}
	proxyTxt := "系统代理:" + g.proxyText()
	switch {
	case g.proxyFail:
		proxyImport = widget.DangerImportance
	case g.proxyOn:
		proxyImport = widget.SuccessImportance
	default:
		proxyImport = widget.LowImportance
	}
	connectTxt := "连接"
	connectIcon := theme.MediaPlayIcon()
	if running {
		connectTxt = "断开"
		connectIcon = theme.MediaStopIcon()
	}
	// 变化检测:所有可见内容拼成指纹,相同则跳过重绘。
	key := strings.Join([]string{
		statusTxt, strconv.Itoa(int(statusImport)),
		speedTxt, uptimeTxt,
		proxyTxt, strconv.Itoa(int(proxyImport)),
		fmt.Sprintf("%v", circleColor), connectTxt,
	}, "\x00")
	if key == g.lastStatus {
		return
	}
	g.lastStatus = key

	g.statusTxt.SetText(statusTxt)
	g.statusTxt.Importance = statusImport
	g.statusCircle.FillColor = circleColor
	g.statusCircle.Refresh()
	g.speedTxt.SetText(speedTxt)
	g.uptimeTxt.SetText(uptimeTxt)
	g.proxyTxt.SetText(proxyTxt)
	g.proxyTxt.Importance = proxyImport
	g.connectBtn.SetText(connectTxt)
	g.connectBtn.SetIcon(connectIcon)
}

func (g *guiApp) proxyText() string {
	if g.proxyFail {
		return "设置失败"
	}
	if g.proxyOn {
		return "开"
	}
	return "关"
}

// latencyText 生成表格延迟列文本。
func latencyText(s *core.Server) string {
	if s.LatencyMS < 0 {
		if s.LatencyErr != "" {
			return "失败"
		}
		return "-"
	}
	return strconv.Itoa(s.LatencyMS) + " ms"
}

// onClose 关闭窗口时的清理:停止核心、恢复系统代理、保存配置。
func (g *guiApp) onClose() {
	if g.stopTicker != nil {
		close(g.stopTicker)
		g.stopTicker = nil
	}
	if g.ticker != nil {
		g.ticker.Stop()
	}
	// 先还原 TUN 路由(设备仍在),再停核心与系统代理。
	if g.tunActive {
		core.TunDown(g.cfg.Settings.TunSubnet)
		g.tunActive = false
	}
	if g.engine.Running() {
		g.engine.Stop()
	}
	if g.proxyOn || g.proxyFail {
		core.RestoreSystemProxy()
	}
	g.proxyOn, g.proxyFail = false, false
	if err := g.cfg.Save(); err != nil {
		g.logView.Append("保存配置失败: " + err.Error())
	}
	// 停掉日志批量刷新,防止 flush 回调在应用退出后操作 UI。
	g.logView.Close()
	g.win.Close()
}

// importClipboard 从剪贴板按行导入服务器。
func (g *guiApp) importClipboard() {
	text := strings.TrimSpace(g.fyneApp.Clipboard().Content())
	if text == "" {
		dialog.ShowInformation("导入剪贴板", "剪贴板为空,请先复制分享链接", g.win)
		return
	}
	ok, dup, fails := g.parseAndImport(text)
	if err := g.cfg.Save(); err != nil {
		dialog.ShowError(err, g.win)
		return
	}
	g.table.Refresh()
	msg := fmt.Sprintf("导入完成:成功 %d,重复 %d,失败 %d", ok, dup, len(fails))
	if len(fails) > 0 {
		msg += "\n\n失败行:\n" + strings.Join(fails, "\n")
	}
	dialog.ShowInformation("导入剪贴板", msg, g.win)
}

// importSubscribe 从订阅 URL 下载并导入服务器。
func (g *guiApp) importSubscribe() {
	ImportURLDialog(g.win, func(url string) {
		if url == "" {
			return
		}
		go func() {
			client := &http.Client{Timeout: 15 * time.Second}
			resp, err := client.Get(url)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("订阅下载失败:%v", err), g.win)
				})
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("读取订阅内容失败:%v", err), g.win)
				})
				return
			}
			fyne.Do(func() {
				ok, dup, fails := g.parseAndImport(string(body))
				if err := g.cfg.Save(); err != nil {
					dialog.ShowError(err, g.win)
					return
				}
				g.table.Refresh()
				msg := fmt.Sprintf("导入完成:成功 %d,重复 %d,失败 %d", ok, dup, len(fails))
				if len(fails) > 0 {
					msg += "\n\n失败行:\n" + strings.Join(fails, "\n")
				}
				dialog.ShowInformation("导入订阅", msg, g.win)
			})
		}()
	})
}

// parseAndImport 解析文本中的分享链接,按 Key 去重加入配置,返回统计。
func (g *guiApp) parseAndImport(text string) (ok, dup int, fails []string) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		srv, err := core.ParseLink(line)
		if err != nil {
			fails = append(fails, line)
			continue
		}
		if g.serverExists(srv) {
			dup++
			continue
		}
		g.cfg.Servers = append(g.cfg.Servers, srv)
		ok++
	}
	return
}

func (g *guiApp) serverExists(s *core.Server) bool {
	for _, existing := range g.cfg.Servers {
		if existing.Key() == s.Key() {
			return true
		}
	}
	return false
}

// addServer 打开添加服务器对话框。
func (g *guiApp) addServer() {
	ManualServerDialog(g.win, g.cfg, nil, func(*core.Server) {
		g.table.Refresh()
	})
}

// editServer 打开编辑服务器对话框。
func (g *guiApp) editServer() {
	if g.selected < 0 {
		dialog.ShowInformation("提示", "请先在列表中选中一个服务器", g.win)
		return
	}
	existing := g.cfg.Servers[g.selected]
	ManualServerDialog(g.win, g.cfg, existing, func(*core.Server) {
		g.table.Refresh()
	})
}

// deleteServer 删除选中的服务器(带确认)。
func (g *guiApp) deleteServer() {
	if g.selected < 0 {
		dialog.ShowInformation("提示", "请先在列表中选中一个服务器", g.win)
		return
	}
	srv := g.cfg.Servers[g.selected]
	ConfirmDialog(g.win, "删除服务器", fmt.Sprintf("确定删除服务器「%s」吗?", srv.Name), func() {
		g.cfg.Servers = append(g.cfg.Servers[:g.selected], g.cfg.Servers[g.selected+1:]...)
		g.selected = -1
		if err := g.cfg.Save(); err != nil {
			dialog.ShowError(err, g.win)
			return
		}
		g.table.Refresh()
	})
}

// speedTest 并发测速所有服务器(超时取自设置 LatencyTimeout,默认 3 秒)。
func (g *guiApp) speedTest() {
	if len(g.cfg.Servers) == 0 {
		dialog.ShowInformation("提示", "还没有服务器,请先导入", g.win)
		return
	}
	if g.testing {
		return
	}
	g.testing = true
	g.speedBtn.Disable()
	g.speedBtn.SetText("测速中...")
	for _, s := range g.cfg.Servers {
		s.LatencyMS = -1
		s.LatencyErr = ""
	}
	g.table.Refresh()

	timeout := time.Duration(g.cfg.Settings.LatencyTimeout) * time.Second
	if g.cfg.Settings.LatencyTimeout <= 0 {
		timeout = 3 * time.Second
	}
	var wg sync.WaitGroup
	for _, s := range g.cfg.Servers {
		wg.Add(1)
		go func(srv *core.Server) {
			defer wg.Done()
			core.TestLatency(srv, timeout)
		}(s)
	}
	go func() {
		wg.Wait()
		fyne.Do(func() {
			g.testing = false
			g.speedBtn.Enable()
			g.speedBtn.SetText("测速")
			g.table.Refresh()
		})
	}()
}

// toggleConnect 在连接与断开之间切换。
func (g *guiApp) toggleConnect() {
	if g.engine.Running() {
		g.disconnect()
		return
	}
	if g.selected < 0 {
		dialog.ShowInformation("提示", "请先在列表中选中一个服务器", g.win)
		return
	}
	g.connectTo(g.cfg.Servers[g.selected])
}

// connectTo 连接指定服务器:启动内嵌核心,按需配置 TUN 路由或系统代理。
func (g *guiApp) connectTo(srv *core.Server) {
	if g.engine.Running() {
		g.disconnect()
	}
	if g.cfg.Settings.TunEnable && core.NeedsRoot() {
		// 提权后仍非 root(罕见,pkexec 被绕过时),保留原错误提示。
		if os.Getenv("V2RAY_GUI_ELEVATED") == "1" {
			dialog.ShowError(fmt.Errorf("TUN 网卡代理需要 root 权限,请用 pkexec ./v2ray-gui 或 sudo -E ./v2ray-gui 启动应用"), g.win)
			return
		}
		// 普通权限:询问是否通过 pkexec 重启提权,重启后自动连接当前节点。
		ConfirmDialog(g.win, "需要管理员权限",
			"TUN 网卡代理需要 root 权限。\n是否通过 pkexec 以管理员权限重启应用?重启后将自动连接当前节点。",
			func() {
				if err := relaunchElevated(srv.ID); err != nil {
					dialog.ShowError(fmt.Errorf("无法通过 pkexec 提权:%v", err), g.win)
					return
				}
				g.logView.Append("正在通过 pkexec 请求管理员权限...")
				g.fyneApp.Quit()
			})
		return
	}
	g.logView.Append("正在连接 " + srv.Name + " (" + srv.Address + ") ...")
	if err := g.engine.Start(srv, g.cfg.Settings); err != nil {
		g.logView.Append("连接失败: " + err.Error())
		dialog.ShowError(fmt.Errorf("连接失败:%v", err), g.win)
		g.updateStatus()
		return
	}
	g.currentSrv = srv
	g.logView.Append("核心已启动,SOCKS:" + strconv.Itoa(g.cfg.Settings.SocksPort) +
		" HTTP:" + strconv.Itoa(g.cfg.Settings.HTTPPort))
	g.cfg.LastServerID = srv.ID
	if err := g.cfg.Save(); err != nil {
		g.logView.Append("保存配置失败: " + err.Error())
	}
	// TUN 网卡级代理:核心在进程内创建设备,系统侧路由用 ip 命令配置,
	// 不需要系统代理。
	if g.cfg.Settings.TunEnable {
		if err := core.TunUp(g.cfg.Settings.TunSubnet); err != nil {
			g.engine.Stop()
			g.currentSrv = nil
			g.logView.Append("TUN 路由配置失败: " + err.Error())
			dialog.ShowError(fmt.Errorf("TUN 路由配置失败:%v", err), g.win)
			g.updateStatus()
			return
		}
		g.tunActive = true
		g.logView.Append("TUN 网卡代理已启用 (tun0),断开时自动还原路由")
		g.proxyOn = false
		g.proxyFail = false
		g.updateStatus()
		return
	}
	if g.cfg.Settings.SetSystemProxy {
		if err := core.SetSystemProxy(g.cfg.Settings); err != nil {
			g.proxyOn = false
			g.proxyFail = true
			g.logView.Append("设置系统代理失败: " + err.Error())
			dialog.ShowInformation("系统代理", "自动设置系统代理失败,可手动配置或到设置中关闭自动设置", g.win)
		} else {
			g.proxyOn = true
			g.proxyFail = false
		}
	} else {
		g.proxyOn = false
		g.proxyFail = false
	}
	g.updateStatus()
}

// disconnect 断开连接:还原 TUN 路由,停止核心,恢复系统代理。
func (g *guiApp) disconnect() {
	g.currentSrv = nil
	if g.tunActive {
		core.TunDown(g.cfg.Settings.TunSubnet)
		g.tunActive = false
		g.logView.Append("TUN 路由已还原")
	}
	if g.engine.Running() {
		g.engine.Stop()
		g.logView.Append("核心已停止")
	}
	if g.proxyOn || g.proxyFail {
		core.RestoreSystemProxy()
	}
	g.proxyOn = false
	g.proxyFail = false
	g.updateStatus()
}

// settingsDialog 打开设置对话框。
func (g *guiApp) settingsDialog() {
	SettingsDialog(g.win, g.cfg, func() {
		g.updateStatus()
	})
}

// aboutDialog 显示关于对话框。
func (g *guiApp) aboutDialog() {
	dialog.ShowInformation("关于",
		"v2rayG v1.0.0\n\n基于 Fyne 内嵌 v2ray-core 的图形化代理客户端\n支持 vmess / vless / trojan / shadowsocks\n界面全中文,类 v2rayN 体验",
		g.win)
}

// formatBytes 将字节数格式化为 B/KB/MB/GB。
func formatBytes(n int64) string {
	f := float64(n)
	switch {
	case f >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", f/(1024*1024*1024))
	case f >= 1024*1024:
		return fmt.Sprintf("%.1f MB", f/(1024*1024))
	case f >= 1024:
		return fmt.Sprintf("%.1f KB", f/1024)
	default:
		return strconv.FormatInt(n, 10) + " B"
	}
}

// formatDuration 将时长格式化为 HH:MM:SS。
func formatDuration(d time.Duration) string {
	total := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d:%02d", total/3600, (total%3600)/60, total%60)
}
