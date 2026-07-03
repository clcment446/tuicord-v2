package widget

import (
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

// Item is one row in an ItemList. Label is the primary text; Badge is optional
// trailing text (an unread count, a mention marker) drawn right-aligned. A zero
// Style falls back to the list's default row or selected style.
type Item struct {
	Label string
	Badge string
	Style screen.Style
}

// ItemList draws and navigates a virtualized list of styled rows. Unlike List,
// each row carries its own label, optional right-aligned badge, and per-row
// style override, so callers can render unread badges, mention counts, and
// muted/inactive rows without post-processing plain strings.
type ItemList struct {
	items         []Item
	selected      int
	offset        int
	style         screen.Style
	selectedStyle screen.Style
	badgeStyle    screen.Style
	onSelect      func(int)
	node          layout.Node
}

// NewItemList returns a list containing items.
func NewItemList(items []Item) *ItemList {
	w := &ItemList{
		selectedStyle: screen.Style{Attrs: screen.Reverse},
		badgeStyle:    screen.Style{Attrs: screen.Bold},
		node:          layout.Node{Grow: 1},
	}
	w.SetItems(items)
	return w
}

// Items returns a copy of the list rows.
func (w *ItemList) Items() []Item {
	if w == nil {
		return nil
	}
	out := make([]Item, len(w.items))
	copy(out, w.items)
	return out
}

// SetItems replaces the list rows and clamps selection.
func (w *ItemList) SetItems(items []Item) {
	if w == nil {
		return
	}
	w.items = append(w.items[:0], items...)
	if len(w.items) == 0 {
		w.selected = 0
		w.offset = 0
		return
	}
	w.selected = clampInt(w.selected, 0, len(w.items)-1)
}

// Selected returns the selected row index, or -1 when the list is empty.
func (w *ItemList) Selected() int {
	if w == nil || len(w.items) == 0 {
		return -1
	}
	return w.selected
}

// SetSelected selects row index after clamping it to the list bounds.
func (w *ItemList) SetSelected(index int) {
	if w == nil || len(w.items) == 0 {
		return
	}
	prev := w.selected
	w.selected = clampInt(index, 0, len(w.items)-1)
	if w.selected != prev && w.onSelect != nil {
		w.onSelect(w.selected)
	}
}

// SetStyle sets the style used for unselected rows.
func (w *ItemList) SetStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.style = style
}

// SetSelectedStyle sets the style used for the selected row.
func (w *ItemList) SetSelectedStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.selectedStyle = style
}

// SetBadgeStyle sets the style used for row badges on unselected rows.
func (w *ItemList) SetBadgeStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.badgeStyle = style
}

// OnSelect registers a callback invoked with the new index whenever the
// selection changes. Passing nil clears the callback.
func (w *ItemList) OnSelect(fn func(int)) {
	if w == nil {
		return
	}
	w.onSelect = fn
}

// CanFocus reports that the list can receive keyboard focus.
func (w *ItemList) CanFocus() bool {
	return w != nil
}

// Measure returns the preferred size for visible list rows.
func (w *ItemList) Measure(avail tui.Size) tui.Size {
	if w == nil {
		return tui.Size{}
	}
	width := 0
	for _, item := range w.items {
		row := text.Width(item.Label)
		if item.Badge != "" {
			row += text.Width(item.Badge) + 1
		}
		width = maxInt(width, row)
	}
	if avail.W > 0 {
		width = minInt(width, avail.W)
	}
	height := len(w.items)
	if avail.H > 0 {
		height = minInt(height, avail.H)
	}
	return tui.Size{W: width, H: height}
}

// Layout returns the layout node for this list.
func (w *ItemList) Layout() *layout.Node {
	if w == nil {
		return nil
	}
	return &w.node
}

// Draw renders only the rows visible in r.
func (w *ItemList) Draw(r screen.Region) {
	if w == nil {
		return
	}
	clear(r, w.style)
	w.ensureSelectedVisible(r.Height())
	for y := 0; y < r.Height(); y++ {
		index := w.offset + y
		if index >= len(w.items) {
			clearLine(r, y, w.style)
			continue
		}
		w.drawRow(r, y, index)
	}
}

func (w *ItemList) drawRow(r screen.Region, y, index int) {
	item := w.items[index]
	selected := index == w.selected
	rowStyle := w.style
	if item.Style != (screen.Style{}) {
		rowStyle = item.Style
	}
	if selected {
		rowStyle = w.selectedStyle
	}
	clearLine(r, y, rowStyle)
	badgeW := 0
	if item.Badge != "" {
		badgeW = text.Width(item.Badge)
	}
	labelWidth := maxInt(r.Width()-badgeW-1, 0)
	if badgeW == 0 {
		labelWidth = r.Width()
	}
	drawText(r, 0, y, text.Truncate(item.Label, labelWidth, text.Ellipsis), rowStyle)
	if badgeW > 0 && badgeW < r.Width() {
		badgeStyle := w.badgeStyle
		if selected {
			badgeStyle = w.selectedStyle
		}
		drawText(r, r.Width()-badgeW, y, item.Badge, badgeStyle)
	}
}

// Handle changes selection for keyboard and wheel events.
func (w *ItemList) Handle(ev tui.Event) bool {
	if w == nil {
		return false
	}
	switch ev := ev.(type) {
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		switch ev.Key {
		case input.KeyUp:
			w.SetSelected(w.selected - 1)
			return true
		case input.KeyDown:
			w.SetSelected(w.selected + 1)
			return true
		case input.KeyHome:
			w.SetSelected(0)
			return true
		case input.KeyEnd:
			w.SetSelected(len(w.items) - 1)
			return true
		case input.KeyPageUp:
			w.SetSelected(w.selected - 10)
			return true
		case input.KeyPageDown:
			w.SetSelected(w.selected + 10)
			return true
		}
	case input.MouseEvent:
		if ev.Kind != input.MouseWheel {
			return false
		}
		switch ev.Btn {
		case input.ButtonWheelUp:
			w.SetSelected(w.selected - 1)
			return true
		case input.ButtonWheelDown:
			w.SetSelected(w.selected + 1)
			return true
		}
	}
	return false
}

func (w *ItemList) ensureSelectedVisible(height int) {
	if height <= 0 || len(w.items) == 0 {
		w.offset = 0
		return
	}
	if w.selected < w.offset {
		w.offset = w.selected
	}
	if w.selected >= w.offset+height {
		w.offset = w.selected - height + 1
	}
	w.offset = clampInt(w.offset, 0, maxInt(len(w.items)-height, 0))
}
