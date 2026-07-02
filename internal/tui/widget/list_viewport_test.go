package widget

import (
	"testing"

	"awesomeProject/internal/tui/screen"
)

func TestListDrawsVisibleSelectedRows(t *testing.T) {
	list := NewList([]string{"zero", "one", "two", "three"})
	list.SetSelected(3)

	buf := screen.NewBuffer(5, 2)
	list.Draw(buf.Clip(buf.Bounds()))

	if got, want := bufferRow(buf, 0), "two  "; got != want {
		t.Fatalf("row 0 = %q, want %q", got, want)
	}
	if got, want := bufferRow(buf, 1), "three"; got != want {
		t.Fatalf("row 1 = %q, want %q", got, want)
	}
	if got := buf.Cell(0, 1).Style.Attrs; got != screen.Reverse {
		t.Fatalf("selected row attrs = %v, want %v", got, screen.Reverse)
	}
}

func TestViewportDrawsScrolledWindow(t *testing.T) {
	viewport := NewViewport()
	viewport.SetLines([]string{"alpha", "beta", "gamma"})
	viewport.SetScroll(1, 1)

	buf := screen.NewBuffer(3, 2)
	viewport.Draw(buf.Clip(buf.Bounds()))

	if got, want := bufferRow(buf, 0), "eta"; got != want {
		t.Fatalf("row 0 = %q, want %q", got, want)
	}
	if got, want := bufferRow(buf, 1), "amm"; got != want {
		t.Fatalf("row 1 = %q, want %q", got, want)
	}
}

func bufferRow(buf *screen.Buffer, y int) string {
	out := ""
	for x := 0; x < buf.Width(); x++ {
		out += buf.Cell(x, y).Content
	}
	return out
}
