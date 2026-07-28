package ui

import (
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

func TestStickerGridNavigatesThreeByThree(t *testing.T) {
	items := make([]widget.Item, 12)
	for i := range items {
		items[i].Label = string(rune('a' + i))
	}
	grid := NewStickerGrid(items, Styles{}, "square")

	grid.Handle(input.KeyEvent{Key: input.KeyRight})
	grid.Handle(input.KeyEvent{Key: input.KeyDown})
	if got := grid.Selected(); got != 4 {
		t.Fatalf("selected = %d, want 4", got)
	}
	grid.Handle(input.KeyEvent{Key: input.KeyPageDown})
	if got := grid.Selected(); got != 9 {
		t.Fatalf("page-down selected = %d, want 9", got)
	}
}

func TestStickerGridRendersConfiguredBordersAndNineItems(t *testing.T) {
	items := make([]widget.Item, 12)
	for i := range items {
		items[i].Label = string(rune('a' + i))
	}
	grid := NewStickerGrid(items, Styles{}, "square")
	buf := tui.New().Render(grid, tui.Size{W: 30, H: 15})
	if got := buf.Cell(0, 0).Content; got != "┌" {
		t.Fatalf("top-left border = %q, want square corner", got)
	}
	if bufferContains(buf, "j") {
		t.Fatal("grid rendered an item beyond the visible 3x3 page")
	}

	grid = NewStickerGrid(items, Styles{}, "double")
	buf = tui.New().Render(grid, tui.Size{W: 30, H: 15})
	if got := buf.Cell(0, 0).Content; got != "╔" {
		t.Fatalf("rounded/double top-left = %q, want double edge glyph", got)
	}
}

func TestStickerGridMouseSelectsCell(t *testing.T) {
	grid := NewStickerGrid([]widget.Item{{Label: "a"}, {Label: "b"}, {Label: "c"}, {Label: "d"}}, Styles{}, "square")
	tui.New().Render(grid, tui.Size{W: 30, H: 15})
	if !grid.Handle(input.MouseEvent{X: 5, Y: 7, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("mouse click was not handled")
	}
	if got := grid.Selected(); got != 4-1 {
		t.Fatalf("mouse selected = %d, want 3", got)
	}
}
