package widget

import (
	"strings"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

// Viewport draws a scrollable set of plain text lines.
type Viewport struct {
	lines   []string
	child   tui.Widget
	offsetY int
	offsetX int
	style   screen.Style
	node    layout.Node
	viewW   int
	viewH   int
}

// NewViewport returns an empty scrollable viewport.
func NewViewport() *Viewport {
	return &Viewport{node: layout.Node{Dir: layout.Column, Grow: 1}}
}

// SetContent replaces the viewport content, splitting it on newlines.
func (w *Viewport) SetContent(content string) {
	if w == nil {
		return
	}
	w.SetLines(strings.Split(content, "\n"))
}

// SetLines replaces the viewport lines.
func (w *Viewport) SetLines(lines []string) {
	if w == nil {
		return
	}
	w.lines = append(w.lines[:0], lines...)
	w.offsetY = clampInt(w.offsetY, 0, maxInt(len(w.lines)-1, 0))
}

// Lines returns a copy of the viewport lines.
func (w *Viewport) Lines() []string {
	if w == nil {
		return nil
	}
	out := make([]string, len(w.lines))
	copy(out, w.lines)
	return out
}

// Scroll returns the horizontal and vertical scroll offsets in cells and rows.
func (w *Viewport) Scroll() (x, y int) {
	if w == nil {
		return 0, 0
	}
	return w.offsetX, w.offsetY
}

// SetScroll sets the horizontal and vertical scroll offsets.
func (w *Viewport) SetScroll(x, y int) {
	if w == nil {
		return
	}
	w.offsetX = maxInt(x, 0)
	if w.child != nil {
		w.offsetY = maxInt(y, 0)
	} else {
		w.offsetY = clampInt(y, 0, maxInt(len(w.lines)-1, 0))
	}
	w.rebuild()
}

// Child returns the retained child widget, if this viewport hosts one.
func (w *Viewport) Child() tui.Widget {
	if w == nil {
		return nil
	}
	return w.child
}

// SetChild makes the viewport host a retained child tree instead of plain text
// lines. The child is laid out at the negative scroll offset and clipped by the
// viewport's region.
func (w *Viewport) SetChild(child tui.Widget) {
	if w == nil {
		return
	}
	w.child = child
	w.rebuild()
}

// Children returns the hosted child for retained-tree traversal.
func (w *Viewport) Children() []tui.Widget {
	if w == nil || w.child == nil {
		return nil
	}
	return []tui.Widget{w.child}
}

// SetStyle sets the style used for viewport text.
func (w *Viewport) SetStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.style = style
}

// CanFocus reports that the viewport can receive keyboard focus for scrolling.
func (w *Viewport) CanFocus() bool {
	return w != nil
}

// Measure returns the preferred size of the current content.
func (w *Viewport) Measure(avail tui.Size) tui.Size {
	if w == nil {
		return tui.Size{}
	}
	if w.child != nil {
		return w.child.Measure(avail)
	}
	width := 0
	for _, line := range w.lines {
		width = maxInt(width, text.Width(line))
	}
	if avail.W > 0 {
		width = minInt(width, avail.W)
	}
	height := len(w.lines)
	if avail.H > 0 {
		height = minInt(height, avail.H)
	}
	return tui.Size{W: width, H: height}
}

// Layout returns the layout node for this viewport.
func (w *Viewport) Layout() *layout.Node {
	if w == nil {
		return nil
	}
	return &w.node
}

// Draw renders only the visible lines.
func (w *Viewport) Draw(r screen.Region) {
	if w == nil {
		return
	}
	w.viewW = r.Width()
	w.viewH = r.Height()
	clear(r, w.style)
	if w.child != nil {
		return
	}
	for y := 0; y < r.Height(); y++ {
		index := w.offsetY + y
		if index >= len(w.lines) {
			break
		}
		drawPaddedText(r, y, visibleFromCell(w.lines[index], w.offsetX, r.Width()), w.style)
	}
}

// Handle scrolls the viewport for keyboard and wheel events.
func (w *Viewport) Handle(ev tui.Event) bool {
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
			w.SetScroll(w.offsetX, w.offsetY-1)
			return true
		case input.KeyDown:
			w.SetScroll(w.offsetX, w.offsetY+1)
			return true
		case input.KeyLeft:
			w.SetScroll(w.offsetX-1, w.offsetY)
			return true
		case input.KeyRight:
			w.SetScroll(w.offsetX+1, w.offsetY)
			return true
		case input.KeyPageUp:
			w.SetScroll(w.offsetX, w.offsetY-10)
			return true
		case input.KeyPageDown:
			w.SetScroll(w.offsetX, w.offsetY+10)
			return true
		case input.KeyHome:
			w.SetScroll(0, 0)
			return true
		case input.KeyEnd:
			w.SetScroll(w.offsetX, len(w.lines)-1)
			return true
		}
	case input.MouseEvent:
		if ev.Kind != input.MouseWheel {
			return false
		}
		switch ev.Btn {
		case input.ButtonWheelUp:
			w.SetScroll(w.offsetX, w.offsetY-1)
			return true
		case input.ButtonWheelDown:
			w.SetScroll(w.offsetX, w.offsetY+1)
			return true
		}
	}
	return false
}

// ScrollExtent implements ScrollModel: the vertical offset, the height drawn
// last frame, and the content height in rows. Before the first Draw the
// viewport size is zero, which renders an attached Scrollbar inert.
func (w *Viewport) ScrollExtent() (offset, viewport, content int) {
	if w == nil {
		return 0, 0, 0
	}
	content = len(w.lines)
	if w.child != nil {
		content = w.child.Measure(tui.Size{W: w.viewW}).H
	}
	return w.offsetY, w.viewH, content
}

// ScrollTo implements ScrollModel by moving the vertical offset.
func (w *Viewport) ScrollTo(offset int) {
	if w == nil {
		return
	}
	w.SetScroll(w.offsetX, offset)
}

func (w *Viewport) rebuild() {
	w.node.Padding = layout.Insets{Top: -w.offsetY, Left: -w.offsetX}
	w.node.Children = nil
	if w.child != nil {
		w.node.Children = []*layout.Node{w.child.Layout()}
	}
}
