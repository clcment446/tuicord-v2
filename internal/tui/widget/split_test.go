package widget

import (
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
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

func TestSplitForwardsUnhandledInputToPane(t *testing.T) {
	field := NewTextInput("")
	split := NewSplit(field, NewText("right")).Basis(8)
	app := tui.New()
	app.Render(split, tui.Size{W: 20, H: 1})

	if !app.Handle(input.PasteEvent{Text: "token"}) {
		t.Fatal("paste was not handled")
	}
	if got := field.Value(); got != "token" {
		t.Fatalf("field value = %q, want token", got)
	}
}

func TestSplitDoesNotForwardMouseToSiblingPane(t *testing.T) {
	left := &mouseRecorder{node: layout.Node{Grow: 1}}
	right := &mouseRecorder{node: layout.Node{Grow: 1}}
	split := NewSplit(left, right).Basis(5)
	app := tui.New()
	app.Render(split, tui.Size{W: 11, H: 1})

	if !app.Handle(input.MouseEvent{X: 6, Y: 0, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("right pane did not receive mouse press")
	}
	if right.handled != 1 {
		t.Fatalf("right handled = %d, want 1", right.handled)
	}
	if left.handled != 0 {
		t.Fatalf("left handled = %d, want 0", left.handled)
	}
}

func TestSplitAltArrowStillResizesBeforeForwarding(t *testing.T) {
	field := NewTextInput("")
	split := NewSplit(field, NewText("right")).Basis(8)

	if !split.Handle(input.KeyEvent{Key: input.KeyLeft, Mods: input.Alt}) {
		t.Fatal("alt-left was not handled")
	}
	if got, want := split.Layout().Children[0].Basis, 7; got != want {
		t.Fatalf("basis = %d, want %d", got, want)
	}
	if got := field.Value(); got != "" {
		t.Fatalf("field value = %q, want empty", got)
	}
}

func TestSplitCollapsesFirstPaneAndExpandsFromToggle(t *testing.T) {
	split := NewSplit(NewText("left"), NewText("right")).Basis(5).CollapsibleFirst()

	split.Basis(0)
	if got := len(split.Children()); got != 1 {
		t.Fatalf("children while collapsed = %d, want 1", got)
	}

	buf := tui.New().Render(split, tui.Size{W: 10, H: 3})
	if got := buf.Cell(0, 1).Content; got != "▶" {
		t.Fatalf("collapsed toggle = %q, want ▶", got)
	}

	if !split.Handle(input.MouseEvent{X: 0, Y: 1, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("collapsed toggle click was not handled")
	}
	if got := len(split.Children()); got != 2 {
		t.Fatalf("children after expand = %d, want 2", got)
	}
}

func TestSplitCollapsesSecondPaneAndExpandsFromToggle(t *testing.T) {
	split := NewSplit(NewText("left"), NewText("right")).Basis(5).CollapsibleSecond()
	split.main = 10

	split.Basis(9)
	if got := len(split.Children()); got != 1 {
		t.Fatalf("children while collapsed = %d, want 1", got)
	}

	buf := tui.New().Render(split, tui.Size{W: 10, H: 3})
	if got := buf.Cell(9, 1).Content; got != "◀" {
		t.Fatalf("collapsed toggle = %q, want ◀", got)
	}

	if !split.Handle(input.MouseEvent{X: 9, Y: 1, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("collapsed toggle click was not handled")
	}
	if got := len(split.Children()); got != 2 {
		t.Fatalf("children after expand = %d, want 2", got)
	}
}

func TestSplitDividerClickCollapsesAndRestoresWithOldSize(t *testing.T) {
	split := NewSplit(NewText("left"), NewText("right")).
		MinFirst(1).Basis(5).CollapsibleFirst()

	// A press-and-release on the divider with no movement is a click: it should
	// collapse the first pane instantly.
	op, ok := split.DragStart(5, 0)
	if !ok {
		t.Fatal("DragStart did not hit divider")
	}
	op.DragEnd(true)
	if !split.collapsedFirst {
		t.Fatal("divider click did not collapse the first pane")
	}
	if got := len(split.Children()); got != 1 {
		t.Fatalf("children while collapsed = %d, want 1", got)
	}

	// Re-clicking the collapsed toggle strip restores the previous size.
	split.main = 10
	if !split.Handle(input.MouseEvent{X: 0, Y: 0, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("collapsed toggle click was not handled")
	}
	if split.collapsedFirst {
		t.Fatal("re-click did not expand the first pane")
	}
	if got, want := split.Layout().Children[0].Basis, 5; got != want {
		t.Fatalf("basis after restore = %d, want %d (old size)", got, want)
	}
}

func TestSplitDividerDragDoesNotCollapse(t *testing.T) {
	split := NewSplit(NewText("left"), NewText("right")).
		MinFirst(1).Basis(5).CollapsibleFirst()

	op, ok := split.DragStart(5, 0)
	if !ok {
		t.Fatal("DragStart did not hit divider")
	}
	op.DragMove(-2, 0) // an actual drag
	op.DragEnd(true)
	if split.collapsedFirst {
		t.Fatal("a drag should resize, not collapse")
	}
	if got, want := split.Layout().Children[0].Basis, 3; got != want {
		t.Fatalf("basis after drag = %d, want %d", got, want)
	}
}

type mouseRecorder struct {
	node    layout.Node
	handled int
}

func (w *mouseRecorder) Measure(avail tui.Size) tui.Size { return avail }
func (w *mouseRecorder) Layout() *layout.Node            { return &w.node }
func (w *mouseRecorder) Draw(screen.Region)              {}
func (w *mouseRecorder) Handle(tui.Event) bool {
	w.handled++
	return true
}
