package tui

import (
	"testing"

	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
)

func TestFocusTraversalSkipsNonFocusableAndWraps(t *testing.T) {
	first := newTestWidget("first", true)
	skipped := newTestWidget("skipped", false)
	second := newTestWidget("second", true)

	var focus FocusManager
	focus.Register(first)
	focus.Register(skipped)
	focus.Register(second)

	if got := focus.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}
	if got := focus.Focused(); !sameWidget(got, first) {
		t.Fatalf("Focused() = %v, want first", got)
	}
	if got := focus.Next(); !sameWidget(got, second) {
		t.Fatalf("Next() = %v, want second", got)
	}
	if got := focus.Next(); !sameWidget(got, first) {
		t.Fatalf("Next() wrapped to %v, want first", got)
	}
	if got := focus.Prev(); !sameWidget(got, second) {
		t.Fatalf("Prev() wrapped to %v, want second", got)
	}
}

func TestFocusReplacePreservesCurrent(t *testing.T) {
	first := newTestWidget("first", true)
	second := newTestWidget("second", true)
	third := newTestWidget("third", true)

	var focus FocusManager
	focus.Replace([]Widget{first, second})
	focus.Set(second)
	focus.Replace([]Widget{first, second, third})

	if got := focus.Focused(); !sameWidget(got, second) {
		t.Fatalf("Focused() = %v, want preserved second", got)
	}
	focus.Replace([]Widget{first, third})
	if got := focus.Focused(); !sameWidget(got, first) {
		t.Fatalf("Focused() = %v, want first after removing second", got)
	}
}

func TestFocusReplaceUsesPreferredWhenNoCurrent(t *testing.T) {
	first := newTestWidget("first", true)
	second := &preferredWidget{testWidget: *newTestWidget("second", true)}

	var focus FocusManager
	focus.Replace([]Widget{first, second})

	if got := focus.Focused(); !sameWidget(got, second) {
		t.Fatalf("Focused() = %v, want preferred second", got)
	}
}

func TestFocusHistoryNavigatesBackAndForward(t *testing.T) {
	first := newTestWidget("first", true)
	second := newTestWidget("second", true)
	third := newTestWidget("third", true)

	var focus FocusManager
	focus.Replace([]Widget{first, second, third})
	focus.Set(second)
	focus.Set(third)

	if got := focus.Back(); !sameWidget(got, second) {
		t.Fatalf("Back() = %v, want second", got)
	}
	if got := focus.Back(); !sameWidget(got, first) {
		t.Fatalf("second Back() = %v, want first", got)
	}
	if got := focus.Forward(); !sameWidget(got, second) {
		t.Fatalf("Forward() = %v, want second", got)
	}
	if got := focus.Forward(); !sameWidget(got, third) {
		t.Fatalf("second Forward() = %v, want third", got)
	}
}

func TestFocusHistoryNewVisitDropsForwardEntries(t *testing.T) {
	first := newTestWidget("first", true)
	second := newTestWidget("second", true)
	third := newTestWidget("third", true)

	var focus FocusManager
	focus.Replace([]Widget{first, second, third})
	focus.Set(second)
	focus.Set(third)
	focus.Back()
	focus.Set(first)

	if got := focus.Forward(); got != nil {
		t.Fatalf("Forward() after new visit = %v, want nil", got)
	}
}

func TestFocusReplacePrunesHistoryAndRemapsCursor(t *testing.T) {
	first := newTestWidget("first", true)
	removed := newTestWidget("removed", true)
	current := newTestWidget("current", true)
	forward := newTestWidget("forward", true)

	var focus FocusManager
	focus.Replace([]Widget{first, removed, current, forward})
	focus.Set(removed)
	focus.Set(current)
	focus.Set(forward)
	focus.Back()
	focus.Replace([]Widget{first, current, forward})

	if len(focus.history) != 3 || focus.historyIndex != 1 {
		t.Fatalf("history len/index = %d/%d, want 3/1", len(focus.history), focus.historyIndex)
	}
	for _, visit := range focus.history {
		if sameWidget(visit, removed) {
			t.Fatal("removed widget retained in history")
		}
	}
	if got := focus.Back(); !sameWidget(got, first) {
		t.Fatalf("Back() = %v, want first", got)
	}
	if got := focus.Forward(); !sameWidget(got, current) {
		t.Fatalf("Forward() = %v, want current", got)
	}
	if got := focus.Forward(); !sameWidget(got, forward) {
		t.Fatalf("second Forward() = %v, want forward", got)
	}
}

func TestFocusRemovePrunesEveryVisitAndClearsBackingReferences(t *testing.T) {
	first := newTestWidget("first", true)
	removed := newTestWidget("removed", true)
	last := newTestWidget("last", true)

	var focus FocusManager
	focus.Replace([]Widget{first, removed, last})
	focus.Set(removed)
	focus.Set(first)
	focus.Set(removed)
	focus.Set(last)
	oldHistoryLen := len(focus.history)

	if !focus.Remove(removed) {
		t.Fatal("Remove returned false")
	}
	for _, visit := range focus.history {
		if sameWidget(visit, removed) {
			t.Fatal("removed widget retained in history")
		}
	}
	backing := focus.history[:oldHistoryLen]
	for i := len(focus.history); i < oldHistoryLen; i++ {
		if backing[i] != nil {
			t.Fatalf("discarded history slot %d retains %v", i, backing[i])
		}
	}
}

func TestFocusRemoveCurrentRecordsReplacement(t *testing.T) {
	first := newTestWidget("first", true)
	second := newTestWidget("second", true)
	third := newTestWidget("third", true)

	var focus FocusManager
	focus.Replace([]Widget{first, second, third})
	focus.Set(second)
	if !focus.Remove(second) {
		t.Fatal("Remove returned false")
	}
	if got := focus.Focused(); !sameWidget(got, third) {
		t.Fatalf("Focused() = %v, want third", got)
	}
	if got := focus.Back(); !sameWidget(got, first) {
		t.Fatalf("Back() = %v, want first", got)
	}
}

func TestFocusReplaceClearsDiscardedRingReferences(t *testing.T) {
	first := newTestWidget("first", true)
	second := newTestWidget("second", true)
	third := newTestWidget("third", true)
	var focus FocusManager
	focus.Replace([]Widget{first, second, third})

	focus.Replace([]Widget{first})
	backing := focus.ring[:3]
	if backing[1] != nil || backing[2] != nil {
		t.Fatalf("discarded ring slots retain widgets: %v", backing)
	}
}

func TestFocusClearReleasesRingAndHistory(t *testing.T) {
	first := newTestWidget("first", true)
	second := newTestWidget("second", true)
	var focus FocusManager
	focus.Replace([]Widget{first, second})
	focus.Set(second)

	focus.Clear()
	if focus.ring != nil || focus.history != nil {
		t.Fatalf("Clear retained slices: ring=%v history=%v", focus.ring, focus.history)
	}
	if focus.current != -1 || focus.historyIndex != -1 || focus.Focused() != nil {
		t.Fatalf("Clear indices = %d/%d, want -1/-1", focus.current, focus.historyIndex)
	}
}

func TestFocusHistoryIsBounded(t *testing.T) {
	first := newTestWidget("first", true)
	second := newTestWidget("second", true)
	var focus FocusManager
	focus.Replace([]Widget{first, second})
	for i := 0; i < focusHistoryLimit+20; i++ {
		if i%2 == 0 {
			focus.Set(second)
		} else {
			focus.Set(first)
		}
	}
	if len(focus.history) != focusHistoryLimit {
		t.Fatalf("history length = %d, want %d", len(focus.history), focusHistoryLimit)
	}
	if focus.historyIndex != focusHistoryLimit-1 {
		t.Fatalf("history index = %d, want %d", focus.historyIndex, focusHistoryLimit-1)
	}
	before := focus.Focused()
	if got := focus.Back(); got == nil || sameWidget(got, before) {
		t.Fatalf("Back() did not navigate bounded history: %v", got)
	}
}

type testWidget struct {
	name     string
	focus    bool
	node     *layout.Node
	children []Widget
	handled  int
}

func newTestWidget(name string, focus bool) *testWidget {
	return &testWidget{name: name, focus: focus, node: &layout.Node{}}
}

func (w *testWidget) Measure(Size) Size    { return Size{} }
func (w *testWidget) Layout() *layout.Node { return w.node }
func (w *testWidget) Draw(screen.Region)   {}
func (w *testWidget) Handle(Event) bool    { w.handled++; return false }
func (w *testWidget) CanFocus() bool       { return w.focus }
func (w *testWidget) Children() []Widget   { return w.children }

type preferredWidget struct {
	testWidget
}

func (w *preferredWidget) PreferredFocus() bool { return true }
