// Package core 提供服务器数据模型、链接解析、v4 配置生成、延迟测试与系统代理。
package core

// TunName 是 TUN 网卡名称,与 v4 配置 services.tun.name 保持一致。
// 设备由常驻 root 助手(internal/helper)创建,网卡与路由也由助手以 root 配置。
const TunName = "tun0"
