package widget

import (
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

func TestModalResizeClampsAndCancels(t *testing.T) {
	m := NewModal("panel", nil)
	m.SetSize(20, 8)
	m.SetPosition(3, 2)
	m.Measure(tui.Size{W: 40, H: 20})
	op, ok := m.ResizeStart(22, 9)
	if !ok {
		t.Fatal("bottom-right corner did not start resize")
	}
	op.DragMove(-99, -99)
	if got := m.Measure(tui.Size{W: 40, H: 20}); got.W != 2 || got.H != 2 {
		t.Fatalf("minimum size = %v, want 2x2", got)
	}
	op.DragEnd(false)
	if got := m.Measure(tui.Size{W: 40, H: 20}); got.W != 20 || got.H != 8 {
		t.Fatalf("cancelled resize = %v, want 20x8", got)
	}
}

func TestModalCollapseChangesReportedBoundsAndRestoresSize(t *testing.T) {
	m := NewModal("panel", nil)
	m.SetSize(20, 8)
	m.SetPosition(3, 2)

	if got := m.Bounds(tui.Size{W: 40, H: 20}); got != (screen.Rect{X: 3, Y: 2, W: 20, H: 8}) {
		t.Fatalf("expanded bounds = %+v", got)
	}
	m.SetCollapsed(true)
	if got := m.Measure(tui.Size{W: 40, H: 20}); got != (tui.Size{W: 20, H: 1}) {
		t.Fatalf("collapsed measure = %+v, want 20x1", got)
	}
	if got := m.Bounds(tui.Size{W: 40, H: 20}); got != (screen.Rect{X: 3, Y: 2, W: 20, H: 1}) {
		t.Fatalf("collapsed bounds = %+v, want title bar only", got)
	}

	m.SetCollapsed(false)
	if got := m.Measure(tui.Size{W: 40, H: 20}); got != (tui.Size{W: 20, H: 8}) {
		t.Fatalf("expanded measure after restore = %+v, want 20x8", got)
	}
}

func TestModalImplicitCollapseKeepsExpandedTitleBarPosition(t *testing.T) {
	m := NewModal("panel", nil)
	m.SetSize(20, 8)
	avail := tui.Size{W: 40, H: 20}
	expanded := m.Bounds(avail)

	m.SetCollapsed(true)
	collapsed := m.Bounds(avail)
	if collapsed.Y != expanded.Y {
		t.Fatalf("collapsed title y = %d, want expanded y %d", collapsed.Y, expanded.Y)
	}
	if collapsed.H != 1 {
		t.Fatalf("collapsed height = %d, want 1", collapsed.H)
	}
}

func TestModalCollapsedDoesNotForwardToHiddenChild(t *testing.T) {
	child := NewTextInput("")
	m := NewModal("panel", child)
	m.SetCollapsed(true)

	if m.Handle(input.PasteEvent{Text: "hidden"}) {
		t.Fatal("collapsed modal forwarded event to hidden child")
	}
	if got := child.Value(); got != "" {
		t.Fatalf("hidden child value = %q, want empty", got)
	}
}

func TestModalOneRowFrameUsesTopCorners(t *testing.T) {
	m := NewModal("", nil)
	m.SetSize(8, 4)
	m.SetCollapsed(true)
	buf := screen.NewBuffer(12, 5)
	m.Draw(buf.Clip(buf.Bounds()))
	rect := m.Bounds(tui.Size{W: 12, H: 5})

	if got := buf.Cell(rect.X, rect.Y).Content; got != "┌" {
		t.Fatalf("one-row left corner = %q, want ┌", got)
	}
	if got := buf.Cell(rect.X+rect.W-1, rect.Y).Content; got != "┐" {
		t.Fatalf("one-row right corner = %q, want ┐", got)
	}
}

func TestModalUsesConfiguredBorderChars(t *testing.T) {
	m := NewModal("", nil)
	m.SetSize(8, 4)
	m.SetBorderChars(BorderChars{TopLeft: "[", TopRight: "]", BottomLeft: "[", BottomRight: "]", Horizontal: "=", Vertical: "!"})
	buf := screen.NewBuffer(12, 5)
	m.Draw(buf.Clip(buf.Bounds()))
	if got := buf.Cell(2, 0).Content; got != "[" {
		t.Fatalf("modal top-left = %q, want [", got)
	}
	if got := buf.Cell(3, 0).Content; got != "=" {
		t.Fatalf("modal top edge = %q, want =", got)
	}
	if got := buf.Cell(2, 1).Content; got != "!" {
		t.Fatalf("modal left edge = %q, want !", got)
	}
}
