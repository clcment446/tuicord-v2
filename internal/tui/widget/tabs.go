package widget

import (
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

// Tab pairs a header label with the widget shown when that tab is active.
type Tab struct {
	// Label is the text drawn in the tab strip.
	Label string
	// Content is the widget revealed when the tab is selected.
	Content tui.Widget
}

// Tabs is a horizontal tab strip above a swappable content area. The strip
// occupies the top row; the active tab's Content fills the rest. Left/Right
// arrows and clicks on the strip change tabs; every other event flows to the
// active content.
//
// Only the active Content participates in layout, hit testing, and focus, so
// hidden tabs cost nothing. Tabs is the substrate for the emoji/GIF/sticker
// picker and the guild-settings panels.
type Tabs struct {
	tabs        []Tab
	active      int
	style       screen.Style
	activeStyle screen.Style
	node        layout.Node
}

// NewTabs returns a tab strip over tabs, with the first tab active.
func NewTabs(tabs []Tab) *Tabs {
	t := &Tabs{
		activeStyle: screen.Style{Attrs: screen.Reverse},
		node:        layout.Node{Dir: layout.Column, Grow: 1, Padding: layout.Insets{Top: 1}},
	}
	t.SetTabs(tabs)
	return t
}

// SetTabs replaces the tabs and clamps the active index.
func (t *Tabs) SetTabs(tabs []Tab) {
	if t == nil {
		return
	}
	t.tabs = append(t.tabs[:0], tabs...)
	t.active = clampInt(t.active, 0, maxInt(len(t.tabs)-1, 0))
	t.rebuild()
}

// Active returns the active tab index, or -1 when there are no tabs.
func (t *Tabs) Active() int {
	if t == nil || len(t.tabs) == 0 {
		return -1
	}
	return t.active
}

// SetActive selects tab index after clamping it to the tab range.
func (t *Tabs) SetActive(index int) {
	if t == nil || len(t.tabs) == 0 {
		return
	}
	t.active = clampInt(index, 0, len(t.tabs)-1)
	t.rebuild()
}

// SetStyle sets the style for inactive tab labels.
func (t *Tabs) SetStyle(s screen.Style) {
	if t != nil {
		t.style = s
	}
}

// SetActiveStyle sets the style for the active tab label.
func (t *Tabs) SetActiveStyle(s screen.Style) {
	if t != nil {
		t.activeStyle = s
	}
}

// Children returns the active tab's content for retained-tree traversal.
func (t *Tabs) Children() []tui.Widget {
	if t == nil || len(t.tabs) == 0 {
		return nil
	}
	content := t.tabs[t.active].Content
	if content == nil {
		return nil
	}
	return []tui.Widget{content}
}

// Measure returns the available size; the active content is sized by layout.
func (t *Tabs) Measure(avail tui.Size) tui.Size {
	if t == nil {
		return tui.Size{}
	}
	return avail
}

// Layout returns the column node with the active content inset below the strip.
func (t *Tabs) Layout() *layout.Node {
	if t == nil {
		return nil
	}
	return &t.node
}

// Draw paints the tab strip on the top row. The active content draws itself.
func (t *Tabs) Draw(r screen.Region) {
	if t == nil || r.Width() <= 0 || r.Height() <= 0 {
		return
	}
	clearLine(r, 0, t.style)
	x := 0
	for i, tab := range t.tabs {
		style := t.style
		if i == t.active {
			style = t.activeStyle
		}
		label := " " + tab.Label + " "
		x = drawText(r, x, 0, label, style)
		if x >= r.Width() {
			break
		}
	}
}

// Handle switches tabs on Left/Right arrows and strip clicks, and forwards all
// other events to the active content when called directly.
func (t *Tabs) Handle(ev tui.Event) bool {
	if t == nil || len(t.tabs) == 0 {
		return false
	}
	if t.handleOwn(ev) {
		return true
	}
	if content := t.tabs[t.active].Content; content != nil {
		return content.Handle(ev)
	}
	return false
}

// HandleBubble switches the tab strip without redispatching to active content.
func (t *Tabs) HandleBubble(ev tui.Event) bool {
	return t != nil && len(t.tabs) > 0 && t.handleOwn(ev)
}

func (t *Tabs) handleOwn(ev tui.Event) bool {
	switch ev := ev.(type) {
	case input.KeyEvent:
		if !ev.Release {
			switch ev.Key {
			case input.KeyLeft:
				t.SetActive(t.active - 1)
				return true
			case input.KeyRight:
				t.SetActive(t.active + 1)
				return true
			}
		}
	case input.MouseEvent:
		if ev.Kind == input.MousePress && ev.Btn == input.ButtonLeft && ev.Y == 0 {
			if idx, ok := t.tabAtX(ev.X); ok {
				t.SetActive(idx)
				return true
			}
		}
	}
	return false
}

// tabAtX maps a strip x-coordinate to a tab index. The layout is deterministic
// from the labels — each tab occupies " Label " — so it needs no render state.
func (t *Tabs) tabAtX(x int) (int, bool) {
	col := 0
	for i, tab := range t.tabs {
		w := text.Width(tab.Label) + 2
		if x >= col && x < col+w {
			return i, true
		}
		col += w
	}
	return 0, false
}

func (t *Tabs) rebuild() {
	t.node.Children = t.node.Children[:0]
	if len(t.tabs) == 0 {
		return
	}
	if content := t.tabs[t.active].Content; content != nil {
		t.node.Children = append(t.node.Children, content.Layout())
	}
}
