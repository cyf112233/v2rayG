package app

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// ScrollEntry 是不带内部滚动容器的输入框,滚轮事件转发给外层滚动容器。
//
// fyne 2.8 的 widget.Entry 默认在内部维护一个 widget.Scroll(ScrollBoth,
// 见 widget/entry.go CreateRenderer:Wrapping 非 TextWrapOff 时内部滚动容器
// 进入对象树)。滚轮事件按"鼠标下最深的 fyne.Scrollable"分发且不冒泡
// (internal/driver/util.go FindObjectAtPositionMatching),内部滚动容器永远
// 先命中并消费事件,导致外层滚动容器(如服务器表单对话框的滚动区)收不到滚轮。
//
// 修复方式:Wrapping=TextWrapOff 且 Scroll=ScrollNone 时,CreateRenderer
// 不会把内部滚动容器加入对象树(entry.go:194),再通过 Scrolled 把事件直接
// 转发给外层滚动容器,方向与钳制逻辑复用 fyne 官方实现(scroller.go:615)。
type ScrollEntry struct {
	widget.Entry
	parent *container.Scroll
}

// NewScrollEntry 创建可转发滚轮事件到外层滚动容器的输入框。
func NewScrollEntry() *ScrollEntry {
	e := &ScrollEntry{}
	e.Wrapping = fyne.TextWrapOff // 关键:配合 ScrollNone 把内部滚动容器移出对象树
	e.Scroll = fyne.ScrollNone    // 关键:与 TextWrapOff 组合后内部滚动容器被隐藏
	e.ExtendBaseWidget(e)
	return e
}

// Scrolled 把滚轮事件转发给外层滚动容器,复用其方向与钳制逻辑。
// parent 未设置(如未放在滚动容器中)时忽略滚轮事件。
func (e *ScrollEntry) Scrolled(ev *fyne.ScrollEvent) {
	if e.parent != nil {
		e.parent.Scrolled(ev)
	}
}

// SetParent 设置外层滚动容器,在表单外层滚动容器创建后调用。
func (e *ScrollEntry) SetParent(s *container.Scroll) { e.parent = s }
