package widget

import (
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

func TestMenuEndAndHomeKeys(t *testing.T) {
	m := NewMenu([]MenuItem{
		{Label: "a"},
		{Label: "b"},
		{Label: "c"},
	})
	m.Handle(key(input.KeyEnd))
	if got := m.Selected(); got != 2 {
		t.Fatalf("End selection = %d, want 2", got)
	}
	m.Handle(key(input.KeyHome))
	if got := m.Selected(); got != 0 {
		t.Fatalf("Home selection = %d, want 0", got)
	}
}

func TestMenuWheelScrollsSelection(t *testing.T) {
	m := NewMenu([]MenuItem{{Label: "a"}, {Label: "b"}})
	m.Handle(input.MouseEvent{Kind: input.MouseWheel, Btn: input.ButtonWheelDown})
	if got := m.Selected(); got != 1 {
		t.Fatalf("after wheel down = %d, want 1", got)
	}
	m.Handle(input.MouseEvent{Kind: input.MouseWheel, Btn: input.ButtonWheelUp})
	if got := m.Selected(); got != 0 {
		t.Fatalf("after wheel up = %d, want 0", got)
	}
}

func TestMenuMotionUpdatesHoverSelection(t *testing.T) {
	m := NewMenu([]MenuItem{{Label: "a"}, {Label: "b"}, {Label: "c"}})
	m.SetAnchor(0, 0)
	buf := screen.NewBuffer(20, 8)
	m.Measure(sizeOf(buf))
	m.Draw(buf.Clip(buf.Bounds()))

	// Hover the third row ("c") at y = 3.
	m.Handle(input.MouseEvent{X: 2, Y: 3, Kind: input.MouseMotion})
	if got := m.Selected(); got != 2 {
		t.Fatalf("hover selection = %d, want 2", got)
	}
}

func TestMenuSettersApplyStyles(t *testing.T) {
	m := NewMenu([]MenuItem{
		{Label: "reply", Key: "R"},
		{Label: "delete", Key: "Del", Danger: true},
	})
	danger := screen.Style{Fg: screen.RGB(9, 9, 9)}
	border := screen.Style{Fg: screen.RGB(1, 1, 1)}
	m.SetStyle(screen.Style{Attrs: screen.Bold})
	m.SetSelectedStyle(screen.Style{Attrs: screen.Reverse})
	m.SetDangerStyle(danger)
	m.SetDisabledStyle(screen.Style{Attrs: screen.Dim})
	m.SetKeyStyle(screen.Style{Attrs: screen.Dim})
	m.SetBorderStyle(border)
	m.SetAnchor(0, 0)

	buf := screen.NewBuffer(20, 8)
	m.Measure(sizeOf(buf))
	m.Draw(buf.Clip(buf.Bounds()))

	if got := buf.Cell(0, 0).Style; got != border {
		t.Fatalf("corner border style = %+v, want %+v", got, border)
	}
	// "delete" is danger (row 1, y=2) and not selected (selection starts at 0).
	if got := buf.Cell(2, 2).Style; got != danger {
		t.Fatalf("danger row style = %+v, want %+v", got, danger)
	}
}

func TestMenuFocusContract(t *testing.T) {
	m := NewMenu([]MenuItem{{Label: "a"}})
	if !m.CanFocus() {
		t.Fatal("menu should be focusable")
	}
	if !m.PreferredFocus() {
		t.Fatal("menu should be the preferred focus owner")
	}
	if m.Layout() == nil {
		t.Fatal("menu Layout should return a node")
	}
}

func TestMenuNilSafe(t *testing.T) {
	var m *Menu
	// None of these should panic on a nil menu.
	m.SetItems(nil)
	m.SetAnchor(1, 1)
	m.OnDismiss(func() {})
	m.SetStyle(screen.Style{})
	if got := m.Selected(); got != -1 {
		t.Fatalf("nil Selected = %d, want -1", got)
	}
	if m.CanFocus() {
		t.Fatal("nil menu is not focusable")
	}
	if m.Layout() != nil {
		t.Fatal("nil Layout should be nil")
	}
	if got := m.Measure(tui.Size{W: 5, H: 5}); got != (tui.Size{}) {
		t.Fatalf("nil Measure = %+v, want zero", got)
	}
	if m.Handle(key(input.KeyEsc)) {
		t.Fatal("nil Handle should not consume")
	}
}
