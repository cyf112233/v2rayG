package app

import (
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// logFlushInterval 日志批量刷新周期:Append 先把行放进 pending 队列,
// 定时器每 250ms 把期间积攒的行一次性追加到 TextGrid。
// 逐行追加每次都会触发 Fyne 全量重绘,是滚动掉帧的主要放大因素,必须批量。
const logFlushInterval = 250 * time.Millisecond

// logMaxRows 日志最大保留行数,超过时裁剪最旧行,避免长时间运行内存与重绘开销无限膨胀。
const logMaxRows = 2000

// LogView 是带时间戳、自动滚动的只读日志视图。
// Append 可能从核心 goroutine 调用:行先入 pending,由定时 flush 在
// Fyne 主线程批量追加;Close 之后不再触碰 UI。
type LogView struct {
	grid   *widget.TextGrid
	scroll *container.Scroll

	mu         sync.Mutex
	pending    []string
	flushTimer *time.Timer
	closed     bool
}

// NewLogView 创建日志视图。
func NewLogView() *LogView {
	grid := widget.NewTextGrid()
	return &LogView{grid: grid, scroll: container.NewScroll(grid)}
}

// Content 返回可放入窗口的滚动容器。
func (lv *LogView) Content() fyne.CanvasObject {
	return lv.scroll
}

// Append 追加一行日志(时间戳在批量 flush 时统一加)。
// 可能从核心 goroutine 调用;Close 之后的行直接丢弃。
func (lv *LogView) Append(line string) {
	lv.mu.Lock()
	defer lv.mu.Unlock()
	if lv.closed {
		return
	}
	lv.pending = append(lv.pending, line)
	if lv.flushTimer == nil {
		lv.flushTimer = time.AfterFunc(logFlushInterval, lv.flush)
	}
}

// flush 取出 pending 中的行,切回主线程批量追加并滚动到底部。
func (lv *LogView) flush() {
	lv.mu.Lock()
	if lv.flushTimer != nil {
		lv.flushTimer.Stop()
		lv.flushTimer = nil
	}
	lines := lv.pending
	lv.pending = nil
	closed := lv.closed
	lv.mu.Unlock()

	if len(lines) == 0 || closed {
		return
	}
	fyne.Do(func() {
		// 回调执行时可能窗口已关闭,再查一次 closed 保证安全。
		lv.mu.Lock()
		stillOpen := !lv.closed
		lv.mu.Unlock()
		if !stillOpen {
			return
		}
		ts := time.Now().Format("15:04:05")
		for _, line := range lines {
			lv.grid.Append("[" + ts + "] " + line)
		}
		// TextGrid 没有 RemoveRow;Rows 是导出字段,渲染器按 Rows 重绘,
		// 直接裁剪最旧行再 Refresh,避免全量重建,简单且不卡。
		if len(lv.grid.Rows) > logMaxRows {
			excess := len(lv.grid.Rows) - logMaxRows
			lv.grid.Rows = append([]widget.TextGridRow(nil), lv.grid.Rows[excess:]...)
			lv.grid.Refresh()
		}
		lv.scroll.ScrollToBottom()
	})
}

// Close 停止定时刷新并标记已关闭。窗口关闭后调用,
// 防止 flush 回调在应用退出后仍操作 UI。
func (lv *LogView) Close() {
	lv.mu.Lock()
	defer lv.mu.Unlock()
	if lv.flushTimer != nil {
		lv.flushTimer.Stop()
		lv.flushTimer = nil
	}
	lv.closed = true
}
