package widget

import (
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

// ScrollModel is the read/write scroll state a Scrollbar reflects and drives.
// Offset is the first visible unit, viewport is how many units fit on screen,
// and content is the total number of units. Viewport and ItemList implement it.
type ScrollModel interface {
	// ScrollExtent returns the current offset, the viewport size, and the
	// content size, all in the same unit (rows for the built-in widgets).
	ScrollExtent() (offset, viewport, content int)
	// ScrollTo moves the first visible unit to offset, clamping as needed.
	ScrollTo(offset int)
}

// Scrollbar is a one-cell-wide vertical slider for a viewport-like widget.
// It draws a proportional thumb over a track, and supports clicking the track
// to page, dragging the thumb, and wheel scrolling. Place it next to the
// widget it controls in a Row container; the widget stays the keyboard-first
// way to scroll, the scrollbar is the mouse affordance and position indicator.
type Scrollbar struct {
	model      ScrollModel
	style      screen.Style
	thumbStyle screen.Style
	node       layout.Node
	height     int
}

// NewScrollbar returns a scrollbar reflecting model.
func NewScrollbar(model ScrollModel) *Scrollbar {
	return &Scrollbar{
		model: model,
		node:  layout.Node{Basis: 1, Min: 1, Max: 1},
	}
}

// SetModel swaps the scroll state the bar reflects.
func (w *Scrollbar) SetModel(model ScrollModel) {
	if w == nil {
		return
	}
	w.model = model
}

// SetStyle sets the style of the track.
func (w *Scrollbar) SetStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.style = style
}

// SetThumbStyle sets the style of the thumb.
func (w *Scrollbar) SetThumbStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.thumbStyle = style
}

// Measure returns a one-cell-wide column filling the available height.
func (w *Scrollbar) Measure(avail tui.Size) tui.Size {
	if w == nil {
		return tui.Size{}
	}
	return tui.Size{W: 1, H: maxInt(avail.H, 0)}
}

// Layout returns the layout node for this scrollbar.
func (w *Scrollbar) Layout() *layout.Node {
	if w == nil {
		return nil
	}
	return &w.node
}

// Draw renders the track and, when the content overflows, the thumb.
func (w *Scrollbar) Draw(r screen.Region) {
	if w == nil {
		return
	}
	w.height = r.Height()
	for y := 0; y < r.Height(); y++ {
		drawText(r, 0, y, "░", w.style)
	}
	top, length, _, ok := w.geometry()
	if !ok {
		return
	}
	for y := top; y < top+length; y++ {
		drawText(r, 0, y, "█", w.thumbStyle)
	}
}

// Handle pages on track clicks and scrolls on wheel events.
func (w *Scrollbar) Handle(ev tui.Event) bool {
	if w == nil || w.model == nil {
		return false
	}
	mouse, isMouse := ev.(input.MouseEvent)
	if !isMouse {
		return false
	}
	offset, viewport, _ := w.model.ScrollExtent()
	switch mouse.Kind {
	case input.MousePress:
		if mouse.Btn != input.ButtonLeft {
			return false
		}
		top, length, _, ok := w.geometry()
		if !ok {
			return false
		}
		switch {
		case mouse.Y < top:
			w.model.ScrollTo(offset - viewport)
			return true
		case mouse.Y >= top+length:
			w.model.ScrollTo(offset + viewport)
			return true
		}
		return false
	case input.MouseWheel:
		switch mouse.Btn {
		case input.ButtonWheelUp:
			w.model.ScrollTo(offset - 1)
			return true
		case input.ButtonWheelDown:
			w.model.ScrollTo(offset + 1)
			return true
		}
	}
	return false
}

// DragStart begins a thumb drag when x,y hit the thumb.
func (w *Scrollbar) DragStart(x, y int) (DragOp, bool) {
	if w == nil || w.model == nil || x != 0 {
		return nil, false
	}
	top, length, _, ok := w.geometry()
	if !ok || y < top || y >= top+length {
		return nil, false
	}
	offset, _, _ := w.model.ScrollExtent()
	return &scrollbarDrag{bar: w, startTop: top, startOffset: offset}, true
}

// geometry returns the thumb position and span for the last drawn height.
// ok is false when nothing overflows, so the bar is inert.
func (w *Scrollbar) geometry() (top, length, maxOffset int, ok bool) {
	if w == nil || w.model == nil || w.height <= 0 {
		return 0, 0, 0, false
	}
	offset, viewport, content := w.model.ScrollExtent()
	if viewport <= 0 || content <= viewport {
		return 0, 0, 0, false
	}
	length = maxInt(w.height*viewport/content, 1)
	length = minInt(length, w.height)
	maxOffset = content - viewport
	maxTop := w.height - length
	offset = clampInt(offset, 0, maxOffset)
	top = (offset*maxTop + maxOffset/2) / maxOffset
	return top, length, maxOffset, true
}

// offsetForThumbTop is the inverse of geometry's thumb placement.
func (w *Scrollbar) offsetForThumbTop(top int) int {
	_, length, maxOffset, ok := w.geometry()
	if !ok {
		return 0
	}
	maxTop := w.height - length
	if maxTop <= 0 {
		return 0
	}
	top = clampInt(top, 0, maxTop)
	return (top*maxOffset + maxTop/2) / maxTop
}

type scrollbarDrag struct {
	bar         *Scrollbar
	startTop    int
	startOffset int
}

// DragMove scrolls the model to follow the dragged thumb.
func (op *scrollbarDrag) DragMove(dx, dy int) {
	if op == nil || op.bar == nil || op.bar.model == nil {
		return
	}
	op.bar.model.ScrollTo(op.bar.offsetForThumbTop(op.startTop + dy))
}

// DragEnd restores the starting offset when the drag is cancelled.
func (op *scrollbarDrag) DragEnd(commit bool) {
	if op == nil || op.bar == nil || op.bar.model == nil || commit {
		return
	}
	op.bar.model.ScrollTo(op.startOffset)
}
