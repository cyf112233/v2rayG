// Package engine 以内嵌方式运行 v2ray-core(进程内启动),替代子进程方案。
package engine

import (
	"bytes"
	"sync"
	"time"

	v2raycore "github.com/v2fly/v2ray-core/v5"
	"github.com/v2fly/v2ray-core/v5/common/log"
	"github.com/v2fly/v2ray-core/v5/features/stats"
	_ "github.com/v2fly/v2ray-core/v5/main/distro/all" // 注册 json 格式与全部 proxy/transport

	"v2ray-gui/internal/core"
)

// Engine 管理一个内嵌的 v2ray-core 实例。
type Engine struct {
	mu        sync.Mutex
	inst      *v2raycore.Instance
	startedAt time.Time
	logf      func(string)
}

// NewEngine 创建引擎。logf 非空时接收核心日志(行内容由调用方加时间戳)。
func NewEngine(logf func(string)) *Engine {
	return &Engine{logf: logf}
}

// Start 生成配置并启动内嵌核心。
// 注意:core.New 期间 app/log 会注册自己的日志 handler(RegisterHandler 是单槽位替换),
// 因此必须在 core.New 之后、inst.Start() 之前注册我们的 handler。
// 启动失败时返回错误并清理实例。实例 Close 后不可复用,再次连接需重新 Start。
func (e *Engine) Start(server *core.Server, st core.Settings) error {
	cfgBytes, err := core.BuildV2rayConfig(server, st)
	if err != nil {
		return err
	}
	cfg, err := v2raycore.LoadConfig("json", bytes.NewReader(cfgBytes))
	if err != nil {
		return err
	}
	inst, err := v2raycore.New(cfg)
	if err != nil {
		return err
	}
	if e.logf != nil {
		log.RegisterHandler(&guiHandler{logf: e.logf})
	}
	if err := inst.Start(); err != nil {
		_ = inst.Close()
		return err
	}
	e.mu.Lock()
	e.inst = inst
	e.startedAt = time.Now()
	e.mu.Unlock()
	return nil
}

// Stop 停止并释放核心实例(幂等)。
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.inst != nil {
		_ = e.inst.Close()
		e.inst = nil
	}
}

// Running 报告核心是否在运行。
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.inst != nil
}

// Uptime 返回本次运行时长;未运行时返回 0。
func (e *Engine) Uptime() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.inst == nil {
		return 0
	}
	return time.Since(e.startedAt)
}

// Traffic 返回累计上行/下行字节数。
// 统计计数器在 instance.Start 之后才注册,查询必须用 GetOrRegisterCounter 并做 nil 检查。
func (e *Engine) Traffic() (up, down int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.inst == nil {
		return 0, 0
	}
	m, ok := e.inst.GetFeature(stats.ManagerType()).(stats.Manager)
	if !ok {
		return 0, 0
	}
	return counterValue(m, "inbound>>>socks-in>>>traffic>>>uplink"),
		counterValue(m, "inbound>>>socks-in>>>traffic>>>downlink")
}

func counterValue(m stats.Manager, name string) int64 {
	c, err := stats.GetOrRegisterCounter(m, name)
	if err != nil || c == nil {
		return 0
	}
	return c.Value()
}

// guiHandler 将核心日志转发给 GUI 回调,丢弃 debug 级别。
type guiHandler struct {
	logf func(string)
}

func (h *guiHandler) Handle(msg log.Message) {
	if gm, ok := msg.(*log.GeneralMessage); ok {
		if gm.Severity > log.Severity_Info {
			return // 丢弃 debug
		}
	}
	if h.logf != nil {
		h.logf(msg.String())
	}
}
