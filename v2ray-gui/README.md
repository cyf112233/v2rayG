# v2rayG

基于 Fyne 的 v2ray-core 图形化代理客户端,类 v2rayN 体验,全中文界面。

核心以源码方式**内嵌** v2ray-core v5(go.mod 的 replace 指向本仓库),点击「连接」即进程内启动,无需外部二进制。

## 功能
- 导入:剪贴板 / 订阅 URL(vmess、vless、trojan、shadowsocks,自动去重)
- 三种路由模式:全局 / 规则 / 直连,切换自动重连
- 一键连接、并发测速、实时流量统计、运行日志、自动连接上次节点
- 系统代理自动设置与恢复(gsettings,GNOME/KDE)
- 系统托盘:单击图标(Windows/macOS)或菜单「显示主界面」显示窗口,菜单含 显示 / 代理开关 / 路由模式 / 退出,连接状态与路由模式实时显示(Linux 需可用 DBus 会话;无 DBus 时自动禁用托盘,不影响使用)
- 单实例:重复启动应用只唤起已有窗口
- 关闭窗口行为可记忆:首次关闭时询问,之后按记忆执行(最小化到托盘 / 退出应用,可在 设置 → 高级设置 → 关闭窗口时 修改)
- 高级设置 21 项:日志级别、域名策略、入站监听、SOCKS 认证、流量探测、Mux、TCP Fast Open、fwmark、强制 DNS、自定义 DNS、TUN 网卡级代理(自动 pkexec 提权)、关闭窗口行为等
- 内置 Noto Sans SC 字体(SIL OFL 1.1,无系统字体依赖)

## 构建
依赖:Go 1.26+、gcc、libgl1-mesa-dev、xorg-dev
```bash
cd v2ray-gui
go build -o v2ray-gui .
./v2ray-gui
```

## 使用
1. 复制分享链接 →「导入剪贴板」;或订阅地址 →「导入订阅」
2. 选中节点 →「测速」
3. 「连接」;TUN 模式自动通过 pkexec 请求 root

## 发行
打 v* tag 触发 GitHub Actions 自动构建(native 构建,不交叉编译):
- Windows:amd64 便携 zip + NSIS 安装程序(自动创建桌面快捷方式)
- Linux:amd64 / arm64,每个架构 tar.gz 独立包 + AppImage + deb + rpm

## 许可
MIT(内嵌 v2ray-core,见 LICENSE);字体 SIL OFL 1.1(assets/fonts/LICENSE)。
