package core

import (
	"net"
	"strconv"
	"time"
)

// TestLatency 通过 TCP 连接测试服务器延迟,结果写入 Server 字段。
// 成功时 LatencyMS 为毫秒耗时且 LatencyErr 清空;失败时 LatencyMS 为 -1 并记录错误。
func TestLatency(s *Server, timeout time.Duration) {
	addr := net.JoinHostPort(s.Address, strconv.Itoa(s.Port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		s.LatencyMS = -1
		s.LatencyErr = err.Error()
		return
	}
	conn.Close()
	s.LatencyMS = int(time.Since(start).Milliseconds())
	s.LatencyErr = ""
}
