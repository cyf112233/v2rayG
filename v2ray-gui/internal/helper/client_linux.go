//go:build linux

package helper

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// OpenTun 请求助手创建 TUN 设备并返回其 fd(经 SCM_RIGHTS 传递)。
//
// 关键时序:助手的响应 JSON 行与 fd 在同一个 sendmsg 中下发(SOCK_STREAM 上
// SCM_RIGHTS 附着在数据字节上)。若先用纯 read 消费数据、再单独 recvmsg 收 fd,
// 内核会把已消费字节上附着的 fd 一并丢弃(实测行为)。因此这里用
// recvmsg(数据缓冲 + oob 缓冲)同时收取两者,累积到 '\n' 解析响应行,并从
// oob 中提取 fd。
func (c *Client) OpenTun(name string) (*os.File, error) {
	conn, err := c.dial()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, errors.New("unexpected socket type")
	}
	rc, err := uc.SyscallConn()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(request{Cmd: "tunopen", Name: name})
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(append(payload, '\n')); err != nil {
		return nil, err
	}
	var (
		respOK  bool
		respErr string
		tunFD   = -1
		recvErr error
	)
	_ = rc.Control(func(fd uintptr) {
		line := make([]byte, 0, 128)
		data := make([]byte, 512)
		oob := make([]byte, 64)
		deadline := time.Now().Add(10 * time.Second)
		for {
			n, oobn, _, _, err := unix.Recvmsg(int(fd), data, oob, unix.MSG_CMSG_CLOEXEC)
			if err != nil {
				if err == unix.EINTR {
					continue
				}
				if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
					if time.Now().After(deadline) {
						recvErr = errors.New("等待助手响应超时")
						return
					}
					time.Sleep(20 * time.Millisecond)
					continue
				}
				recvErr = err
				return
			}
			if n > 0 {
				line = append(line, data[:n]...)
			}
			if oobn > 0 {
				if msgs, err := unix.ParseSocketControlMessage(oob[:oobn]); err == nil {
					for _, m := range msgs {
						if fds, err := unix.ParseUnixRights(&m); err == nil && len(fds) > 0 {
							tunFD = fds[0]
						}
					}
				}
			}
			if idx := bytes.IndexByte(line, '\n'); idx >= 0 {
				var resp response
				if err := json.Unmarshal(line[:idx], &resp); err != nil {
					recvErr = err
					return
				}
				respOK = resp.OK
				respErr = resp.Err
				if tunFD >= 0 || !respOK {
					return
				}
				// 响应 ok 但 fd 尚未到达:继续收。
			}
			if time.Now().After(deadline) {
				recvErr = errors.New("收取 TUN fd 超时")
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
	if recvErr != nil {
		return nil, recvErr
	}
	if !respOK {
		return nil, errors.New(respErr)
	}
	if tunFD < 0 {
		return nil, errors.New("未收到 TUN fd")
	}
	return os.NewFile(uintptr(tunFD), "v2rayg-tun"), nil
}
