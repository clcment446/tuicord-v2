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

func TestSplitResponsiveHideRemovesPaneReservationAndSurvivesRebuild(t *testing.T) {
	split := NewSplit(NewText("chat"), NewText("members")).
		Basis(200).
		MinSecond(20).
		HideSecondBelow(120)

	wide := layout.Solve(split.Layout(), 120, 1)
	first := split.Layout().Children[0]
	second := split.Layout().Children[1]
	if got := wide[first].W; got != 99 {
		t.Fatalf("first pane at threshold width = %d, want 99", got)
	}
	if got := wide[second].W; got != 20 {
		t.Fatalf("second pane at threshold width = %d, want 20", got)
	}

	// Basis rebuilds the generated wrappers. The responsive policy must remain
	// on the wrapper and remove both its minimum width and the divider gap.
	split.Basis(180)
	first = split.Layout().Children[0]
	second = split.Layout().Children[1]
	narrow := layout.Solve(split.Layout(), 119, 1)
	if _, ok := narrow[second]; ok {
		t.Fatal("second pane remained in layout below responsive threshold")
	}
	if got := narrow[first].W; got != 119 {
		t.Fatalf("first pane below threshold = %d, want full 119", got)
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

func TestSplitDividerUsesConfiguredThemeStyle(t *testing.T) {
	style := screen.Style{Fg: screen.RGB(1, 2, 3), Bg: screen.RGB(4, 5, 6)}
	split := NewSplit(NewText("left"), NewText("right")).Basis(5)
	split.SetStyle(style)

	buf := tui.New().Render(split, tui.Size{W: 12, H: 1})
	got := buf.Cell(5, 0).Style
	if got.Fg != style.Fg || got.Bg != style.Bg {
		t.Fatalf("divider style = %+v, want %+v", got, style)
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

func TestSplitIsNotKeyboardFocusable(t *testing.T) {
	split := NewSplit(NewText("left"), NewText("right"))
	if split.CanFocus() {
		t.Fatal("split divider should not be selectable by Tab")
	}

	split.Basis(0)
	if split.CanFocus() {
		t.Fatal("collapsed split toggle should not be selectable by Tab")
	}
}

func TestTabAndShiftTabTraverseInputsWithoutSelectingSplitSeparators(t *testing.T) {
	first := NewTextInput("first")
	second := NewTextInput("second")
	root := NewSplit(first, second).Basis(10)
	app := tui.New()
	app.Render(root, tui.Size{W: 30, H: 1})

	if got := app.Focus.Len(); got != 2 {
		t.Fatalf("focus ring length = %d, want 2 inputs only", got)
	}
	if got := app.Focus.Focused(); got != first {
		t.Fatal("first input should receive initial focus")
	}

	if !app.Handle(input.KeyEvent{Key: input.KeyTab}) {
		t.Fatal("Tab should be handled while an input is focused")
	}
	if got := app.Focus.Focused(); got != second {
		t.Fatal("Tab should move focus to the second input")
	}
	if !app.Handle(input.KeyEvent{Key: input.KeyTab, Mods: input.Shift}) {
		t.Fatal("Shift+Tab should be handled while an input is focused")
	}
	if got := app.Focus.Focused(); got != first {
		t.Fatal("Shift+Tab should move focus back to the first input")
	}
}

func TestFocusableSplitCanBeActivatedWithEnterAndSpace(t *testing.T) {
	split := NewSplit(NewText("left"), NewText("right")).Basis(5).CollapsibleFirst()
	app := tui.New(tui.WithFocusableSplits(true))
	app.Render(split, tui.Size{W: 12, H: 2})

	if got := app.Focus.Focused(); got != split {
		t.Fatal("focusable split should receive focus")
	}
	if !app.Handle(input.KeyEvent{Key: input.KeyRune, Rune: ' '}) {
		t.Fatal("Space should collapse a focused split")
	}
	if len(split.Children()) != 1 {
		t.Fatal("Space did not collapse the split")
	}
	if !app.Handle(input.KeyEvent{Key: input.KeyEnter}) {
		t.Fatal("Enter should expand a focused split")
	}
	if len(split.Children()) != 2 {
		t.Fatal("Enter did not expand the split")
	}
}

func TestFocusableSecondSplitCanBeCollapsedWithSpace(t *testing.T) {
	split := NewSplit(NewText("left"), NewText("right")).Basis(5).CollapsibleSecond()
	app := tui.New(tui.WithFocusableSplits(true))
	app.Render(split, tui.Size{W: 12, H: 2})

	if !app.Handle(input.KeyEvent{Key: input.KeyRune, Rune: ' '}) {
		t.Fatal("Space should collapse a focused second split")
	}
	if len(split.Children()) != 1 {
		t.Fatal("Space did not collapse the second split")
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

// The composed UI sets row sizing (Basis/Grow) on a split's root node — e.g.
// the accounts|composer row is pinned to the composer height. A divider drag or
// collapse triggers rebuild, which must not reset that external sizing (issue:
// dragging the account picker divider made its row grow to half the screen).
func TestSplitRebuildPreservesExternalRootSizing(t *testing.T) {
	split := NewSplit(NewText("accounts"), NewText("composer")).
		Basis(16).MinFirst(8).MaxFirst(28).CollapsibleFirst()
	root := split.Layout()
	root.Basis = 9
	root.Grow = 0

	op, ok := split.DragStart(16, 0)
	if !ok {
		t.Fatal("DragStart did not hit divider")
	}
	op.DragMove(4, 0)
	op.DragEnd(true)

	if root.Basis != 9 || root.Grow != 0 {
		t.Fatalf("root sizing after drag = basis %d grow %v, want basis 9 grow 0", root.Basis, root.Grow)
	}

	split.toggleCollapse()
	split.expand()
	if root.Basis != 9 || root.Grow != 0 {
		t.Fatalf("root sizing after collapse cycle = basis %d grow %v, want basis 9 grow 0", root.Basis, root.Grow)
	}
}
