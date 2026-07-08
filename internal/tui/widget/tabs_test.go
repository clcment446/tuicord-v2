package widget

import (
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

func TestTabsArrowsSwitchTabs(t *testing.T) {
	tabs := NewTabs([]Tab{
		{Label: "Emoji", Content: NewText("emoji")},
		{Label: "GIF", Content: NewText("gif")},
		{Label: "Sticker", Content: NewText("sticker")},
	})
	if got := tabs.Active(); got != 0 {
		t.Fatalf("initial active = %d, want 0", got)
	}
	tabs.Handle(key(input.KeyRight))
	if got := tabs.Active(); got != 1 {
		t.Fatalf("after Right = %d, want 1", got)
	}
	tabs.Handle(key(input.KeyRight))
	tabs.Handle(key(input.KeyRight)) // clamps at last
	if got := tabs.Active(); got != 2 {
		t.Fatalf("after clamping = %d, want 2", got)
	}
	tabs.Handle(key(input.KeyLeft))
	if got := tabs.Active(); got != 1 {
		t.Fatalf("after Left = %d, want 1", got)
	}
}

func TestTabsClickSelectsTab(t *testing.T) {
	tabs := NewTabs([]Tab{
		{Label: "Emoji", Content: NewText("emoji")}, // " Emoji "  -> x 0..6
		{Label: "GIF", Content: NewText("gif")},     // " GIF "    -> x 7..11
		{Label: "Sticker", Content: NewText("sticker")},
	})
	// Click within the "GIF" range (x = 8, y = 0).
	if !tabs.Handle(input.MouseEvent{X: 8, Y: 0, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("strip click not handled")
	}
	if got := tabs.Active(); got != 1 {
		t.Fatalf("clicked tab = %d, want 1", got)
	}
}

func TestTabsClickBelowStripFallsThroughToContent(t *testing.T) {
	// A click off the strip (y > 0) must not switch tabs; it goes to content.
	tabs := NewTabs([]Tab{
		{Label: "Emoji", Content: NewText("emoji")},
		{Label: "GIF", Content: NewText("gif")},
	})
	tabs.Handle(input.MouseEvent{X: 8, Y: 3, Btn: input.ButtonLeft, Kind: input.MousePress})
	if got := tabs.Active(); got != 0 {
		t.Fatalf("content-area click changed tab to %d, want 0", got)
	}
}

func TestTabsChildrenReturnsActiveContent(t *testing.T) {
	emoji := NewText("emoji")
	gif := NewText("gif")
	tabs := NewTabs([]Tab{{Label: "Emoji", Content: emoji}, {Label: "GIF", Content: gif}})

	if kids := tabs.Children(); len(kids) != 1 || kids[0] != emoji {
		t.Fatalf("active children = %v, want [emoji]", kids)
	}
	tabs.SetActive(1)
	if kids := tabs.Children(); len(kids) != 1 || kids[0] != gif {
		t.Fatalf("after switch children = %v, want [gif]", kids)
	}
}

func TestTabsForwardsEventsToActiveContent(t *testing.T) {
	got := ""
	sink := &eventSink{onKey: func(r rune) { got = string(r) }}
	tabs := NewTabs([]Tab{{Label: "One", Content: sink}})

	tabs.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'k'})
	if got != "k" {
		t.Fatalf("content received %q, want k", got)
	}
}

func TestTabsDrawHighlightsActive(t *testing.T) {
	active := screen.Style{Attrs: screen.Reverse}
	tabs := NewTabs([]Tab{{Label: "A", Content: NewText("a")}, {Label: "B", Content: NewText("b")}})
	tabs.SetActiveStyle(active)
	tabs.SetActive(1)

	buf := screen.NewBuffer(12, 3)
	tabs.Draw(buf.Clip(buf.Bounds()))

	// " A " occupies x 0..2; " B " starts at x 3. The 'B' glyph sits at x 4.
	if got := buf.Cell(4, 0).Style; got != active {
		t.Fatalf("active tab style = %+v, want %+v", got, active)
	}
}

func TestTabsEmptyAndNilSafe(t *testing.T) {
	empty := NewTabs(nil)
	if got := empty.Active(); got != -1 {
		t.Fatalf("empty Active = %d, want -1", got)
	}
	if empty.Handle(key(input.KeyRight)) {
		t.Fatal("empty tabs should not consume arrows")
	}

	var nilT *Tabs
	if nilT.Active() != -1 {
		t.Fatal("nil Active should be -1")
	}
	if nilT.Layout() != nil {
		t.Fatal("nil Layout should be nil")
	}
	nilT.SetActive(2) // must not panic
	nilT.SetStyle(screen.Style{})
}

// eventSink is a minimal leaf widget that records key runes.
type eventSink struct {
	onKey func(rune)
	node  layout.Node
}

func (s *eventSink) Measure(a tui.Size) tui.Size { return a }
func (s *eventSink) Layout() *layout.Node        { return &s.node }
func (s *eventSink) Draw(screen.Region)          {}
func (s *eventSink) Handle(ev tui.Event) bool {
	if k, ok := ev.(input.KeyEvent); ok && k.Key == input.KeyRune {
		if s.onKey != nil {
			s.onKey(k.Rune)
		}
		return true
	}
	return false
}
