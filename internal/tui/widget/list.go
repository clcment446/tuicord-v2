package widget

import (
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

// List draws and navigates a virtualized list of rows.
type List struct {
	items         []string
	selected      int
	offset        int
	style         screen.Style
	selectedStyle screen.Style
	node          layout.Node
}

// NewList returns a list containing items.
func NewList(items []string) *List {
	w := &List{
		selectedStyle: screen.Style{Attrs: screen.Reverse},
		node:          layout.Node{Grow: 1},
	}
	w.SetItems(items)
	return w
}

// Items returns a copy of the list rows.
func (w *List) Items() []string {
	if w == nil {
		return nil
	}
	out := make([]string, len(w.items))
	copy(out, w.items)
	return out
}

// SetItems replaces the list rows and clamps selection.
func (w *List) SetItems(items []string) {
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
func (w *List) Selected() int {
	if w == nil || len(w.items) == 0 {
		return -1
	}
	return w.selected
}

// SetSelected selects row index after clamping it to the list bounds.
func (w *List) SetSelected(index int) {
	if w == nil || len(w.items) == 0 {
		return
	}
	w.selected = clampInt(index, 0, len(w.items)-1)
}

// SetStyle sets the style used for unselected rows.
func (w *List) SetStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.style = style
}

// SetSelectedStyle sets the style used for the selected row.
func (w *List) SetSelectedStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.selectedStyle = style
}

// CanFocus reports that the list can receive keyboard focus.
func (w *List) CanFocus() bool {
	return w != nil
}

// Measure returns the preferred size for visible list rows.
func (w *List) Measure(avail tui.Size) tui.Size {
	if w == nil {
		return tui.Size{}
	}
	width := 0
	for _, item := range w.items {
		width = maxInt(width, text.Width(item))
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
func (w *List) Layout() *layout.Node {
	if w == nil {
		return nil
	}
	return &w.node
}

// Draw renders only the rows visible in r.
func (w *List) Draw(r screen.Region) {
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
		style := w.style
		if index == w.selected {
			style = w.selectedStyle
		}
		drawPaddedText(r, y, w.items[index], style)
	}
}

// Handle changes selection for keyboard and wheel events.
func (w *List) Handle(ev tui.Event) bool {
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

func (w *List) ensureSelectedVisible(height int) {
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
