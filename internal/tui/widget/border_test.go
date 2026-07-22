package widget

import (
	"testing"

	"awesomeProject/internal/tui/screen"
)

func TestBorderUsesFocusStyleWhenFocused(t *testing.T) {
	border := NewBorder(NewText("body"))
	normal := screen.Style{Fg: screen.RGB(1, 2, 3)}
	focused := screen.Style{Fg: screen.RGB(4, 5, 6)}
	border.SetStyle(normal)
	border.SetFocusStyle(focused)
	border.SetFocused(true)

	buf := screen.NewBuffer(8, 3)
	border.Draw(buf.Clip(buf.Bounds()))

	if got := buf.Cell(0, 0).Style; got != focused {
		t.Fatalf("focused border style = %+v, want %+v", got, focused)
	}
}

func TestBorderOneRowFrameUsesTopCorners(t *testing.T) {
	border := NewBorder(nil)
	buf := screen.NewBuffer(8, 1)
	border.Draw(buf.Clip(buf.Bounds()))

	if got := buf.Cell(0, 0).Content; got != "┌" {
		t.Fatalf("one-row left corner = %q, want ┌", got)
	}
	if got := buf.Cell(7, 0).Content; got != "┐" {
		t.Fatalf("one-row right corner = %q, want ┐", got)
	}
}
