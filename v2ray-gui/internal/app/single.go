package app

// 单实例机制:第一个实例在 127.0.0.1:<port> 上监听;后续实例连上端口,
// 发送 "show" 让已有实例显示主窗口,然后自己退出。Windows/Linux 通用(TCP)。

import (
	"net"
	"strconv"
	"time"
)

const singlePort = 10880

// acquireSingle 尝试成为主实例。返回 true 表示当前进程是主实例(需继续监听);
// false 表示已有实例在运行(已通知其显示窗口),调用方应立即退出。
func acquireSingle(show chan<- struct{}) bool {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(singlePort))
	// 先尝试连接:能连上说明已有主实例,通知它显示窗口。
	if c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond); err == nil {
		_, _ = c.Write([]byte("show\n"))
		_ = c.Close()
		return false
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// 监听失败:可能恰好被另一进程抢占,再试一次连接。
		if c, err2 := net.DialTimeout("tcp", addr, 500*time.Millisecond); err2 == nil {
			_, _ = c.Write([]byte("show\n"))
			_ = c.Close()
			return false
		}
		return true // 无法确认,按主实例继续,端口只用于唤起,失败不影响使用
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				buf := make([]byte, 64)
				if n, _ := c.Read(buf); n > 0 {
					select {
					case show <- struct{}{}:
					default:
					}
				}
			}()
		}
	}()
	return true
}
