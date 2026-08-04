# v2rayG

基于 Fyne 的 v2ray-core 图形化代理客户端,类 v2rayN 体验,全中文界面。

核心以源码方式**内嵌** v2ray-core v5(go.mod 的 replace 指向本仓库),点击「连接」即进程内启动,无需外部二进制。

## 功能
- 导入:剪贴板 / 订阅 URL(vmess、vless、trojan、shadowsocks,自动去重)
- 三种路由模式:全局 / 规则 / 直连,切换自动重连
- 一键连接、并发测速、实时流量统计、运行日志、自动连接上次节点
- 系统代理自动设置与恢复(gsettings,GNOME/KDE)
- 高级设置 20 项:日志级别、域名策略、入站监听、SOCKS 认证、流量探测、Mux、TCP Fast Open、fwmark、强制 DNS、自定义 DNS、TUN 网卡级代理(自动 pkexec 提权)等
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
打 v* tag 触发 GitHub Actions 自动构建:
- Windows:amd64 / arm64 / 386(便携 zip)
- Linux:amd64 / arm64 / arm / 386(二进制 zip)
- deb / rpm 包:amd64 / arm64,安装自动注册桌面快捷方式与图标

## 许可
MIT(内嵌 v2ray-core,见 LICENSE);字体 SIL OFL 1.1(assets/fonts/LICENSE)。
