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
