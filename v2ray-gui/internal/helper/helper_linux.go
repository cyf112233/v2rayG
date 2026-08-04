//go:build linux

package helper

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Run 是助手主进程入口(由 pkexec 以 root 启动,无 GUI)。
// 若已有助手在监听则直接退出(防重复常驻);否则重建 socket 并进入 accept 循环。
func Run() {
	uid, ok := helperUID()
	if !ok {
		os.Exit(1)
	}
	// 已有助手存活:直接退出,避免重复常驻 root 进程。
	if conn, err := net.Dial("unix", SocketPath); err == nil {
		_ = conn.Close()
		os.Exit(0)
	}
	_ = os.Remove(SocketPath)
	ln, err := net.Listen("unix", SocketPath)
	if err != nil {
		elog("监听 %s 失败:%v", SocketPath, err)
		os.Exit(1)
	}
	// 0666:同一用户的图形应用以普通权限连接。
	if err := os.Chmod(SocketPath, 0o666); err != nil {
		elog("chmod %s 失败:%v", SocketPath, err)
	}
	devs := newDeviceStore()
	idle := newIdleTimer()
	elog("v2rayg helper 已启动 pid=%d uid=%d socket=%s", os.Getpid(), uid, SocketPath)
	for {
		conn, err := ln.Accept()
		if err != nil {
			elog("accept:%v", err)
			continue
		}
		go serve(conn, uid, devs, idle)
	}
}

// helperUID 从 V2RAYG_UID 环境变量读取允许连接的调用者 uid。
func helperUID() (int, bool) {
	raw := os.Getenv("V2RAYG_UID")
	if raw == "" {
		elog("缺少 V2RAYG_UID 环境变量")
		return 0, false
	}
	uid, err := strconv.Atoi(raw)
	if err != nil || uid < 0 {
		elog("V2RAYG_UID 无效:%q", raw)
		return 0, false
	}
	return uid, true
}

// serve 处理一条连接:校验 SO_PEERCRED 后按命令分发。
func serve(conn net.Conn, wantUID int, devs *deviceStore, idle *idleTimer) {
	defer conn.Close()
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return
	}
	var peer *unix.Ucred
	_ = raw.Control(func(fd uintptr) {
		peer, _ = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if peer == nil || int(peer.Uid) != wantUID {
		elog("拒绝来自 uid=%d 的连接", peerUid(peer))
		return
	}
	line, err := readLine(conn)
	if err != nil {
		elog("读取请求失败:%v", err)
		return
	}
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		elog("请求解析失败:%s", string(line))
		return
	}
	elog("收到命令 %s", req.Cmd)
	idle.touch()
	switch req.Cmd {
	case "ping":
		writeResp(conn, response{OK: true})
	case "tunopen":
		handleTunOpen(conn, req.Name, devs)
	case "run":
		handleRun(conn, req.Args)
	case "quit":
		writeResp(conn, response{OK: true})
		elog("收到 quit,退出")
		os.Exit(0)
	default:
		writeResp(conn, response{OK: false, Err: "未知命令:" + req.Cmd})
	}
}

// handleTunOpen 创建(或复用)同名 TUN 设备,并把 fd 随 JSON 响应一并下发。
// 复用同名设备:应用断开后设备仍由助手持有,重连时直接复用,无需重复创建。
func handleTunOpen(conn net.Conn, name string, devs *deviceStore) {
	fd := devs.get(name)
	if fd >= 0 {
		elog("tunopen %s 复用已有 fd=%d", name, fd)
	} else {
		var err error
		fd, err = openTun(name)
		if err != nil {
			elog("openTun 失败:%v", err)
			writeResp(conn, response{OK: false, Err: err.Error()})
			return
		}
		devs.put(name, fd)
		elog("tunopen %s 新建 fd=%d", name, fd)
	}
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		writeResp(conn, response{OK: false, Err: err.Error()})
		return
	}
	var rawfd int
	_ = raw.Control(func(f uintptr) { rawfd = int(f) })
	payload, err := json.Marshal(response{OK: true})
	if err != nil {
		writeResp(conn, response{OK: false, Err: err.Error()})
		return
	}
	// JSON 行与 fd 在同一个 sendmsg 中下发,保证"先读到响应行、后收到 fd"。
	oob := unix.UnixRights(fd)
	if err := unix.Sendmsg(rawfd, append(payload, '\n'), oob, nil, 0); err != nil {
		elog("下发 tun fd 失败:%v", err)
	}
}

// handleRun 以 root 执行 ip 命令,合并输出返回。
func handleRun(conn net.Conn, args []string) {
	cmd := exec.Command(ipPath(), args...)
	out, err := cmd.CombinedOutput()
	resp := response{Out: string(out)}
	if err != nil {
		resp.OK = false
		resp.Err = err.Error()
	} else {
		resp.OK = true
	}
	writeResp(conn, resp)
}

// openTun 打开 /dev/net/tun 并通过 TUNSETIFF 创建指定名称的 TUN 设备。
func openTun(name string) (int, error) {
	if name == "" || len(name) > unix.IFNAMSIZ {
		return -1, fmt.Errorf("无效的网卡名 %q", name)
	}
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("打开 /dev/net/tun 失败:%w", err)
	}
	// 40 字节 ifr:前 16 字节设备名,16-18 字节 flags(小端)。
	var ifr [40]byte
	copy(ifr[:], name)
	*(*uint16)(unsafe.Pointer(&ifr[16])) = unix.IFF_TUN | unix.IFF_NO_PI
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(unix.TUNSETIFF), uintptr(unsafe.Pointer(&ifr[0]))); errno != 0 {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("TUNSETIFF %s 失败:%v", name, errno)
	}
	return fd, nil
}

// writeResp 向连接写入一行 JSON 响应。
func writeResp(conn net.Conn, resp response) {
	payload, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_, _ = conn.Write(append(payload, '\n'))
}

// deviceStore 保存已创建的 tun fd(防重复创建/防 fd 号被复用),同名复用。
type deviceStore struct {
	mu sync.Mutex
	m  map[string]int
}

func newDeviceStore() *deviceStore {
	return &deviceStore{m: make(map[string]int)}
}

func (s *deviceStore) get(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fd, ok := s.m[name]; ok {
		return fd
	}
	return -1 // 注意:不能用 map 零值 0 当"未找到",0 可能是合法 fd
}

func (s *deviceStore) put(name string, fd int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[name] = fd
}

// idleTimer 实现空闲自动退出:30 分钟无请求则整个助手进程退出。
type idleTimer struct {
	mu    sync.Mutex
	timer *time.Timer
}

func newIdleTimer() *idleTimer {
	t := &idleTimer{}
	t.timer = time.AfterFunc(idleTimeout, func() { os.Exit(0) })
	return t
}

func (t *idleTimer) touch() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.timer.Reset(idleTimeout)
}

// ipPath 定位 ip 命令(含 root PATH 兜底)。
func ipPath() string {
	if p, err := exec.LookPath("ip"); err == nil {
		return p
	}
	for _, p := range []string{"/usr/sbin/ip", "/sbin/ip", "/usr/bin/ip", "/bin/ip"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "ip"
}

// elog 写 stderr(由主程序侧捕获或丢弃,不影响功能)。
func elog(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[v2rayg-helper] "+format+"\n", args...)
}

func peerUid(p *unix.Ucred) int {
	if p == nil {
		return -1
	}
	return int(p.Uid)
}
