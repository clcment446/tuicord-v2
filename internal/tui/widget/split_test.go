package widget

import (
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/tui"
)

func TestSplitBasisClampsAndDragCanCancel(t *testing.T) {
	split := NewSplit(nil, nil).MinFirst(3).MaxFirst(5).Basis(99)
	if got, want := split.Layout().Children[0].Basis, 5; got != want {
		t.Fatalf("basis = %d, want %d", got, want)
	}

	op, ok := split.DragStart(5, 0)
	if !ok {
		t.Fatal("DragStart did not hit divider")
	}
	op.DragMove(-10, 0)
	if got, want := split.Layout().Children[0].Basis, 3; got != want {
		t.Fatalf("basis after drag = %d, want %d", got, want)
	}
	op.DragEnd(false)
	if got, want := split.Layout().Children[0].Basis, 5; got != want {
		t.Fatalf("basis after cancel = %d, want %d", got, want)
	}
}

func TestSplitGivesPaneChildFullWidth(t *testing.T) {
	field := NewTextInput("")
	field.Handle(input.PasteEvent{Text: "abcde"})
	field.SetCursor(0)
	split := NewSplit(field, NewText("right")).Basis(5)

	buf := tui.New().Render(split, tui.Size{W: 12, H: 1})
	if got := buf.Cell(4, 0).Content; got != "e" {
		t.Fatalf("first pane was not full width; cell 4 = %q", got)
	}
}
