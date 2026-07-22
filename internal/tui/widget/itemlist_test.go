package widget

import (
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
)

func TestItemListMousePressSelectsRow(t *testing.T) {
	list := NewItemList([]Item{
		{Label: "guild one"},
		{Label: "guild two"},
		{Label: "guild three"},
	})
	var selected []int
	list.OnSelect(func(index int) {
		selected = append(selected, index)
	})

	if !list.Handle(input.MouseEvent{Y: 1, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("mouse press was not handled")
	}
	if got := list.Selected(); got != 1 {
		t.Fatalf("selected = %d, want 1", got)
	}
	if len(selected) != 1 || selected[0] != 1 {
		t.Fatalf("onSelect calls = %v, want [1]", selected)
	}

	if !list.Handle(input.MouseEvent{Y: 1, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("repeat mouse press was not handled")
	}
	if len(selected) != 2 || selected[1] != 1 {
		t.Fatalf("repeat onSelect calls = %v, want second call for row 1", selected)
	}
}

func TestItemListRightClickCapturesContext(t *testing.T) {
	list := NewItemList([]Item{{Label: "a"}, {Label: "b"}, {Label: "c"}})
	var selected []int
	list.OnSelect(func(index int) { selected = append(selected, index) })

	// Right-click records the row but does not consume the event (so the shell
	// can intercept it) and does not change selection.
	if list.Handle(input.MouseEvent{Y: 2, Btn: input.ButtonRight, Kind: input.MousePress}) {
		t.Fatal("right-click should not be consumed by the list")
	}
	if len(selected) != 0 {
		t.Fatalf("right-click should not fire onSelect, got %v", selected)
	}
	idx, ok := list.TakeContext()
	if !ok || idx != 2 {
		t.Fatalf("TakeContext = (%d, %v), want (2, true)", idx, ok)
	}
	if _, ok := list.TakeContext(); ok {
		t.Fatal("TakeContext should clear after first read")
	}
}

func TestItemListRightClickOutOfRangeNoContext(t *testing.T) {
	list := NewItemList([]Item{{Label: "a"}})
	list.Handle(input.MouseEvent{Y: 5, Btn: input.ButtonRight, Kind: input.MousePress})
	if _, ok := list.TakeContext(); ok {
		t.Fatal("out-of-range right-click should not record a context row")
	}
}

func TestItemListDragDropsRows(t *testing.T) {
	list := NewItemList([]Item{{Label: "a"}, {Label: "b"}, {Label: "c"}})
	var from, to int
	list.SetDrag(func(index int) bool { return index != 1 }, nil, func(start, end int) {
		from = start
		to = end
	})

	op, ok := list.DragStart(0, 0)
	if !ok {
		t.Fatal("first row did not start drag")
	}
	op.DragMove(0, 2)
	op.DragEnd(true)
	if from != 0 || to != 2 || list.Selected() != 2 {
		t.Fatalf("drop = %d,%d selected=%d", from, to, list.Selected())
	}
	if _, ok := list.DragStart(0, 1); ok {
		t.Fatal("blocked row started drag")
	}
}

func TestItemListSetItemsInvalidatesActiveDrag(t *testing.T) {
	list := NewItemList([]Item{{Label: "a"}, {Label: "b"}, {Label: "c"}})
	drops := 0
	list.SetDrag(nil, nil, func(int, int) { drops++ })

	op, ok := list.DragStart(0, 0)
	if !ok {
		t.Fatal("first row did not start drag")
	}
	op.DragMove(0, 2)
	list.SetItems([]Item{{Label: "replacement"}})

	// Pointer capture can still deliver motion and release after the owner has
	// rebuilt the rows. Both events must ignore the stale drag indices.
	op.DragMove(0, 10)
	op.DragEnd(true)
	if drops != 0 {
		t.Fatalf("stale drag committed %d drops, want 0", drops)
	}
	if got := list.Selected(); got != 0 {
		t.Fatalf("selected row after replacement = %d, want 0", got)
	}
	buf := screen.NewBuffer(16, 1)
	list.Draw(buf.Clip(buf.Bounds()))
	if got := bufferRow(buf, 0); got != "replacement     " {
		t.Fatalf("row after replacement = %q", got)
	}
}

func TestItemListDragWithoutMotionClicks(t *testing.T) {
	list := NewItemList([]Item{{Label: "a"}, {Label: "b"}})
	selected := -1
	list.OnSelect(func(index int) { selected = index })
	list.SetDrag(nil, nil, func(int, int) {})

	op, ok := list.DragStart(0, 1)
	if !ok {
		t.Fatal("row did not start drag")
	}
	op.DragEnd(true)
	if selected != 1 || list.Selected() != 1 {
		t.Fatalf("click selected=%d row=%d", selected, list.Selected())
	}
}

func TestItemListDrawsDragSpace(t *testing.T) {
	list := NewItemList([]Item{{Label: "alpha"}, {Label: "beta"}, {Label: "gamma"}})
	list.SetDrag(nil, nil, func(int, int) {})
	op, ok := list.DragStart(0, 0)
	if !ok {
		t.Fatal("row did not start drag")
	}
	op.DragMove(0, 2)

	buf := screen.NewBuffer(8, 3)
	list.Draw(buf.Clip(buf.Bounds()))
	if got := bufferRow(buf, 0); got != "beta    " {
		t.Fatalf("shifted row = %q", got)
	}
	if got := bufferRow(buf, 1); got != "gamma   " {
		t.Fatalf("shifted row = %q", got)
	}
	if got := bufferRow(buf, 2); got != "> alpha " {
		t.Fatalf("drag row = %q", got)
	}

	op.DragEnd(false)
	buf = screen.NewBuffer(8, 3)
	list.Draw(buf.Clip(buf.Bounds()))
	if got := bufferRow(buf, 0); got != "alpha   " {
		t.Fatalf("restored row = %q", got)
	}
}

func TestItemListSetSelectedSilentDoesNotNotify(t *testing.T) {
	list := NewItemList([]Item{
		{Label: "one"},
		{Label: "two"},
	})
	var selected []int
	list.OnSelect(func(index int) {
		selected = append(selected, index)
	})

	list.SetSelectedSilent(1)

	if got := list.Selected(); got != 1 {
		t.Fatalf("selected = %d, want 1", got)
	}
	if len(selected) != 0 {
		t.Fatalf("onSelect calls = %v, want none", selected)
	}
}

func TestItemListEnterSelectsInitiallySelectedRow(t *testing.T) {
	list := NewItemList([]Item{{Label: "first"}, {Label: "second"}})
	selected := -1
	list.OnSelect(func(index int) {
		selected = index
	})

	if !list.Handle(input.KeyEvent{Key: input.KeyEnter}) {
		t.Fatal("Enter should be handled")
	}
	if selected != 0 {
		t.Fatalf("Enter selected row = %d, want 0", selected)
	}
}

func TestItemListNavigationDoesNotActivateRows(t *testing.T) {
	list := NewItemList([]Item{{Label: "first"}, {Label: "second"}})
	selected := []int{}
	list.OnSelect(func(index int) { selected = append(selected, index) })

	if !list.Handle(input.KeyEvent{Key: input.KeyDown}) {
		t.Fatal("Down should be handled")
	}
	if got := list.Selected(); got != 1 {
		t.Fatalf("selected = %d, want 1", got)
	}
	if len(selected) != 0 {
		t.Fatalf("navigation activated rows: %v", selected)
	}

	if !list.Handle(input.MouseEvent{Btn: input.ButtonWheelUp, Kind: input.MouseWheel}) {
		t.Fatal("wheel up should be handled")
	}
	if got := list.Selected(); got != 0 {
		t.Fatalf("wheel selected = %d, want 0", got)
	}
	if len(selected) != 0 {
		t.Fatalf("scrolling activated rows: %v", selected)
	}
}

func TestItemListVimJKMovesWithoutActivating(t *testing.T) {
	list := NewItemList([]Item{{Label: "first"}, {Label: "second"}})
	if list.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'j'}) {
		t.Fatal("j was handled before Vim navigation was enabled")
	}
	list.SetVimNavigation(true)
	activated := 0
	list.OnSelect(func(int) { activated++ })
	if !list.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'j'}) || list.Selected() != 1 {
		t.Fatal("j did not move down")
	}
	if !list.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'k'}) || list.Selected() != 0 {
		t.Fatal("k did not move up")
	}
	if activated != 0 {
		t.Fatalf("Vim navigation activated %d rows", activated)
	}
}

func TestItemListVimFocusCanConsumeCollapsedRow(t *testing.T) {
	list := NewItemList([]Item{{Label: "collapsed"}})
	list.SetVimNavigation(true)
	calls := 0
	list.OnVimFocus(func(forward bool) bool {
		calls++
		return forward
	})
	if !list.HandleVimFocus(true) {
		t.Fatal("forward Vim focus did not consume local unfold")
	}
	if list.HandleVimFocus(false) {
		t.Fatal("backward Vim focus should have fallen through")
	}
	if calls != 2 {
		t.Fatalf("callback calls = %d, want 2", calls)
	}
}
