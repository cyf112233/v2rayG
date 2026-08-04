// Package helper 实现常驻 root 助手:主程序通过 pkexec 一次性认证启动一个无 GUI
// 的 root 子进程(本二进制以 V2RAYG_HELPER=1 环境变量进入,见 main.go),此后
// TUN 设备创建、网卡/路由配置等特权操作全部通过 /run/v2rayg-helper.sock 上的
// JSON 行协议交给该助手执行,应用自身保持非 root 运行。助手空闲 30 分钟自动退出。
//
// 协议:请求与响应均为单行 JSON;tunopen 的响应之后经 SCM_RIGHTS 附送 TUN fd
// (响应行与 fd 在同一 sendmsg 中下发,客户端逐字节读取响应行,不会预读吞掉 fd)。
package helper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"time"
)

// SocketPath 是助手监听的 unix socket 路径。
const SocketPath = "/run/v2rayg-helper.sock"

// idleTimeout 是助手空闲自动退出时间:30 分钟无请求即退出,避免常驻 root 进程。
const idleTimeout = 30 * time.Minute

// request 是客户端发给助手的请求(JSON 行)。
type request struct {
	Cmd  string   `json:"cmd"`
	Name string   `json:"name"`
	Args []string `json:"args"`
}

// response 是助手返回的响应(JSON 行)。
type response struct {
	OK  bool   `json:"ok"`
	Err string `json:"err,omitempty"`
	Out string `json:"out,omitempty"`
}

// Client 是主程序侧的助手客户端。每个请求独立建立一条短连接,避免连接状态复杂。
type Client struct {
	path string
}

func (c *Client) dial() (net.Conn, error) {
	return net.Dial("unix", c.path)
}

// do 发送一个请求并读取 JSON 行响应(不涉及 fd 传输的通用路径)。
func (c *Client) do(req request) (*response, error) {
	conn, err := c.dial()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return doOnConn(conn, req)
}

// doOnConn 在给定连接上发送请求并读取响应;OpenTun 复用它并保留连接收取 fd。
func doOnConn(conn net.Conn, req request) (*response, error) {
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(append(payload, '\n')); err != nil {
		return nil, err
	}
	line, err := readLine(conn)
	if err != nil {
		return nil, err
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("解析助手响应失败: %s", string(line))
	}
	return &resp, nil
}

// Ping 探测助手是否存活。
func (c *Client) Ping() error {
	_, err := c.do(request{Cmd: "ping"})
	return err
}

// Run 请求助手以 root 执行一条 ip 命令,返回合并输出。
func (c *Client) Run(args ...string) (string, error) {
	resp, err := c.do(request{Cmd: "run", Args: args})
	if err != nil {
		return "", err
	}
	if !resp.OK {
		return resp.Out, fmt.Errorf("ip %v: %s%s", args, resp.Err, resp.Out)
	}
	return resp.Out, nil
}

// Quit 请求助手退出。应用正常退出时不调用,让助手空闲超时自动退出,
// 以保证"一次认证,后续免密"。
func (c *Client) Quit() error {
	_, err := c.do(request{Cmd: "quit"})
	return err
}

// EnsureClient 确保助手在运行并返回可用客户端:先尝试直连;失败则通过
// pkexec 启动助手(首次会弹出系统认证框,这是唯一一次密码),随后轮询
// 等待 socket 就绪,最长 10 秒。
func EnsureClient() (*Client, error) {
	c := &Client{path: SocketPath}
	if err := c.Ping(); err == nil {
		return c, nil
	}
	if _, err := exec.LookPath("pkexec"); err != nil {
		return nil, fmt.Errorf("未找到 pkexec,无法启动提权助手:%w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	// pkexec 会清洗环境;助手只需要 V2RAYG_HELPER 与 V2RAYG_UID 两个变量。
	cmd := exec.Command("pkexec", "env",
		"V2RAYG_HELPER=1",
		fmt.Sprintf("V2RAYG_UID=%d", os.Getuid()),
		self,
	)
	cmd.Stdout, cmd.Stderr = nil, nil // 交给 pkexec 自己的认证界面
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("无法启动 pkexec:%w", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := c.Ping(); err == nil {
			return c, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, errors.New("提权助手启动超时(可能未完成系统认证)")
}

// readLine 逐字节读取一行(以 '\n' 结尾),不经过 bufio,避免预读吞掉
// SCM_RIGHTS 附加数据(仅 tunopen 响应携带)。
func readLine(r io.Reader) ([]byte, error) {
	line := make([]byte, 0, 256)
	one := make([]byte, 1)
	for {
		n, err := r.Read(one)
		if n > 0 {
			if one[0] == '\n' {
				return line, nil
			}
			line = append(line, one[0])
			if len(line) > 1<<20 {
				return nil, errors.New("响应过长")
			}
		}
		if err != nil {
			if err == io.EOF && len(line) > 0 {
				return line, nil
			}
			return nil, err
		}
	}
}
