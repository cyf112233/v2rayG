package app

import (
	"log"
	"os"
	"path/filepath"
	"sync/atomic"

	"fyne.io/systray"
)

// trayRouteLabels 是托盘"路由模式"子菜单的固定顺序(与 routeModeLabel 的取值对应)。
var trayRouteLabels = []string{"直连", "规则", "全局"}

// Tray 管理系统托盘:单击图标或菜单"显示主界面"显示窗口;
// 右键菜单含 显示 / 代理开关(标题带状态)/ 路由模式三选一(勾选)/ 退出。
type Tray struct {
	ready      atomic.Bool                  // 托盘就绪(菜单已建立)
	connItem   *systray.MenuItem            // 代理开关(标题带状态)
	routeItems map[string]*systray.MenuItem // 直连/规则/全局
	onShow     func()
	onToggle   func()
	onRoute    func(string)
	onQuit     func()
	icon       []byte

	// onReadySync 在托盘就绪后同步一次当前状态(连接状态 + 路由模式勾选),由 app 层注入。
	onReadySync func()
	// lastKey 是上次刷新的内容指纹,内容不变时跳过重复更新。
	lastKey string
}

// NewTray 创建托盘实例。回调在托盘事件 goroutine 中调用,内部已做空值保护;
// 回调内如需操作 UI,请用 fyne.Do 切回主线程。
func NewTray(onShow, onToggle func(), onRoute func(string), onQuit func(), icon []byte) *Tray {
	return &Tray{
		onShow:   onShow,
		onToggle: onToggle,
		onRoute:  onRoute,
		onQuit:   onQuit,
		icon:     icon,
	}
}

// StartTray 启动托盘并阻塞直到 StopTray/Quit。
// 无可用 DBus 会话时跳过(记日志),保证应用照常运行、不 panic 不阻塞。
func (t *Tray) StartTray() {
	if !dbusAvailable() {
		log.Println("无 DBus 会话,系统托盘不可用")
		return
	}
	systray.Run(t.onReady, nil)
}

// dbusAvailable 判断是否存在可用的会话总线:优先看 DBUS_SESSION_BUS_ADDRESS,
// 其次看 XDG_RUNTIME_DIR/bus(systemd 用户会话的常见布局,未设环境变量时托盘仍可用)。
func dbusAvailable() bool {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" {
		return true
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		if _, err := os.Stat(filepath.Join(dir, "bus")); err == nil {
			return true
		}
	}
	return false
}

// onReady 在托盘就绪后设置图标与菜单,并启动各菜单项的点击监听。
func (t *Tray) onReady() {
	systray.SetIcon(t.icon)
	systray.SetTitle("v2rayG")
	systray.SetTooltip("v2rayG - V2Ray 图形代理客户端")

	showItem := systray.AddMenuItem("显示主界面", "显示主界面")
	t.connItem = systray.AddMenuItem("代理: 未连接", "代理开关")
	routeMenu := systray.AddMenuItem("路由模式", "切换路由模式")
	t.routeItems = make(map[string]*systray.MenuItem, len(trayRouteLabels))
	for _, label := range trayRouteLabels {
		t.routeItems[label] = routeMenu.AddSubMenuItemCheckbox(label, "", false)
	}
	systray.AddSeparator()
	quitItem := systray.AddMenuItem("退出应用", "退出应用")

	t.ready.Store(true)
	if t.onReadySync != nil {
		t.onReadySync()
	}

	// 单击托盘图标显示主界面(Windows/macOS 生效;Linux 的 StatusNotifierItem
	// 实现不分发单击回调,由菜单"显示主界面"承担)。
	systray.SetOnTapped(t.safeShow)

	go func() {
		for range showItem.ClickedCh {
			t.safeShow()
		}
	}()
	go func() {
		for range t.connItem.ClickedCh {
			t.safeCall(t.onToggle)
		}
	}()
	for _, label := range trayRouteLabels {
		item := t.routeItems[label]
		go func(label string, item *systray.MenuItem) {
			for range item.ClickedCh {
				t.safeCall(func() { t.onRoute(label) })
			}
		}(label, item)
	}
	go func() {
		for range quitItem.ClickedCh {
			t.safeCall(t.onQuit)
		}
	}()
}

// safeCall 调用回调;回调未设置时跳过(ready 早于 app 绑定回调的防御)。
func (t *Tray) safeCall(f func()) {
	if f != nil {
		f()
	}
}

// safeShow 触发显示主界面的回调。
func (t *Tray) safeShow() {
	t.safeCall(t.onShow)
}

// refreshTray 更新托盘菜单状态:代理开关标题 + 路由模式勾选。
// 未就绪或内容未变化时跳过,可在任意 goroutine 调用。
func (t *Tray) refreshTray(connState, route string) {
	if !t.ready.Load() || t.connItem == nil {
		return
	}
	key := connState + "\x00" + route
	if key == t.lastKey {
		return
	}
	t.lastKey = key
	t.connItem.SetTitle("代理: " + connState)
	for _, label := range trayRouteLabels {
		item := t.routeItems[label]
		if item == nil {
			continue
		}
		if label == route {
			item.Check()
		} else {
			item.Uncheck()
		}
	}
}

// StopTray 退出托盘。仅在托盘已启动时调用,避免无 DBus 时库内部对 nil 连接 Close 的 panic。
func (t *Tray) StopTray() {
	if t.ready.Load() {
		systray.Quit()
	}
}
