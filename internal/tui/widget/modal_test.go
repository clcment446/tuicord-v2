package widget

import (
	"testing"

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
