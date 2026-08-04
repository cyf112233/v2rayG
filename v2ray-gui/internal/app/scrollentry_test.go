package app

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// scrollTestWindow 构造一个 300x150 的小窗口:外层滚动容器 + 若干输入框,
// 输入框总高度超过视口(10 个输入框约 380px > 150px),用于验证滚轮事件分发。
func scrollTestWindow(t *testing.T, entries []fyne.CanvasObject) (*container.Scroll, fyne.Window) {
	t.Helper()
	test.NewApp()
	scroll := container.NewScroll(container.NewVBox(entries...))
	w := test.NewWindow(scroll)
	w.Resize(fyne.NewSize(300, 150))
	return scroll, w
}

// entryCenter 返回输入框在 canvas 上的中心坐标(用于 test.Scroll 的命中测试)。
func entryCenter(e fyne.CanvasObject) fyne.Position {
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(e)
	return pos.Add(fyne.NewPos(e.Size().Width/2, e.Size().Height/2))
}

// TestPlainEntrySwallowsScroll 对照用例:普通 widget.Entry 内部自带滚动容器,
// 滚轮事件被内部容器消费(不冒泡),外层滚动容器不会移动——证明 bug 存在。
func TestPlainEntrySwallowsScroll(t *testing.T) {
	entries := make([]fyne.CanvasObject, 10)
	for i := range entries {
		entries[i] = widget.NewEntry()
	}
	scroll, w := scrollTestWindow(t, entries)
	defer w.Close()

	test.Scroll(w.Canvas(), entryCenter(entries[0]), 0, -100)
	if scroll.Offset.Y != 0 {
		t.Fatalf("普通 Entry 场景外层滚动不应移动,got Offset.Y=%v", scroll.Offset.Y)
	}
}

// TestScrollEntryPassesScrollToOuter 修复用例:ScrollEntry 把滚轮事件转发给
// 外层滚动容器,滚轮在输入框上滚动时外层正常滚动——证明修复有效。
func TestScrollEntryPassesScrollToOuter(t *testing.T) {
	entries := make([]fyne.CanvasObject, 10)
	for i := range entries {
		entries[i] = NewScrollEntry()
	}
	scroll, w := scrollTestWindow(t, entries)
	defer w.Close()
	// 与 ManualServerDialog 一致:外层滚动容器创建后统一 SetParent。
	for _, e := range entries {
		e.(*ScrollEntry).SetParent(scroll)
	}

	test.Scroll(w.Canvas(), entryCenter(entries[0]), 0, -100)
	if scroll.Offset.Y <= 0 {
		t.Fatalf("ScrollEntry 场景滚轮后外层 Offset.Y 应为正,got %v", scroll.Offset.Y)
	}
}

// TestScrollEntryForwardsDirect 直接调用 ScrollEntry.Scrolled,验证事件确实被
// 转发到外层滚动容器(不依赖命中测试,保证转发逻辑正确性)。
func TestScrollEntryForwardsDirect(t *testing.T) {
	entries := make([]fyne.CanvasObject, 5)
	scrollEntries := make([]*ScrollEntry, 5)
	for i := range entries {
		scrollEntries[i] = NewScrollEntry()
		entries[i] = scrollEntries[i]
	}
	scroll, w := scrollTestWindow(t, entries)
	defer w.Close()
	for _, e := range scrollEntries {
		e.SetParent(scroll)
	}

	before := scroll.Offset.Y
	scrollEntries[0].Scrolled(&fyne.ScrollEvent{Scrolled: fyne.Delta{DX: 0, DY: -100}})
	if scroll.Offset.Y == before {
		t.Fatal("ScrollEntry.Scrolled 未把滚轮事件转发到外层滚动容器")
	}
	if scroll.Offset.Y <= 0 {
		t.Fatalf("向下滚动后外层 Offset.Y 应为正,got %v", scroll.Offset.Y)
	}
}
