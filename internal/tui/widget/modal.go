package widget

import (
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

// Modal is a centered overlay with a draggable title bar.
type Modal struct {
	child          tui.Widget
	title          string
	x, y           int
	w, h           int
	screenW        int
	screenH        int
	last           screen.Rect
	explicitOrigin bool
	style          screen.Style
	node           layout.Node
}

// NewModal returns a modal overlay around child.
func NewModal(title string, child tui.Widget) *Modal {
	m := &Modal{title: title, child: child, w: 40, h: 10}
	m.rebuild()
	return m
}

// Child returns the contained widget.
func (w *Modal) Child() tui.Widget {
	if w == nil {
		return nil
	}
	return w.child
}

// SetChild replaces the contained widget.
func (w *Modal) SetChild(child tui.Widget) {
	if w == nil {
		return
	}
	w.child = child
	w.rebuild()
}

// Children returns the modal child for retained-tree traversal.
func (w *Modal) Children() []tui.Widget {
	if w == nil || w.child == nil {
		return nil
	}
	return []tui.Widget{w.child}
}

// SetSize sets the modal size in cells.
func (w *Modal) SetSize(width, height int) {
	if w == nil {
		return
	}
	w.w = maxInt(width, 2)
	w.h = maxInt(height, 2)
}

// SetPosition sets the modal top-left position in cells.
func (w *Modal) SetPosition(x, y int) {
	if w == nil {
		return
	}
	w.explicitOrigin = true
	w.x = maxInt(x, 0)
	w.y = maxInt(y, 0)
	if w.screenW > 0 {
		w.x = clampInt(w.x, 0, maxInt(w.screenW-w.w, 0))
	}
	if w.screenH > 0 {
		w.y = clampInt(w.y, 0, maxInt(w.screenH-w.h, 0))
	}
}

// Position returns the modal top-left position in cells.
func (w *Modal) Position() (x, y int) {
	if w == nil {
		return 0, 0
	}
	return w.x, w.y
}

// Bounds returns the modal rectangle clamped within avail.
func (w *Modal) Bounds(avail tui.Size) screen.Rect {
	if w == nil {
		return screen.Rect{}
	}
	width := minInt(w.w, maxInt(avail.W, 0))
	height := minInt(w.h, maxInt(avail.H, 0))
	x := w.x
	y := w.y
	if !w.explicitOrigin {
		x = maxInt((avail.W-width)/2, 0)
		y = maxInt((avail.H-height)/2, 0)
	}
	x = clampInt(x, 0, maxInt(avail.W-width, 0))
	y = clampInt(y, 0, maxInt(avail.H-height, 0))
	return screen.Rect{X: x, Y: y, W: width, H: height}
}

// SetStyle sets the style used for the modal frame.
func (w *Modal) SetStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.style = style
}

// Measure returns the configured modal size clamped to avail.
func (w *Modal) Measure(avail tui.Size) tui.Size {
	if w == nil {
		return tui.Size{}
	}
	rect := w.Bounds(avail)
	w.screenW = avail.W
	w.screenH = avail.H
	w.last = rect
	w.node.Padding = layout.Insets{
		Top:    rect.Y + 1,
		Left:   rect.X + 1,
		Right:  maxInt(avail.W-(rect.X+rect.W)+1, 0),
		Bottom: maxInt(avail.H-(rect.Y+rect.H)+1, 0),
	}
	return tui.Size{W: minInt(w.w, avail.W), H: minInt(w.h, avail.H)}
}

// Layout returns the modal child layout inset by the frame.
func (w *Modal) Layout() *layout.Node {
	if w == nil {
		return nil
	}
	return &w.node
}

// Draw renders the modal frame into r.
func (w *Modal) Draw(r screen.Region) {
	if w == nil || r.Width() <= 0 || r.Height() <= 0 {
		return
	}
	rect := w.Bounds(tui.Size{W: r.Width(), H: r.Height()})
	w.screenW = r.Width()
	w.screenH = r.Height()
	w.last = rect
	for x := rect.X; x < rect.X+rect.W; x++ {
		r.Set(x, rect.Y, styled("─", w.style))
		r.Set(x, rect.Y+rect.H-1, styled("─", w.style))
	}
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		r.Set(rect.X, y, styled("│", w.style))
		r.Set(rect.X+rect.W-1, y, styled("│", w.style))
	}
	r.Set(rect.X, rect.Y, styled("┌", w.style))
	r.Set(rect.X+rect.W-1, rect.Y, styled("┐", w.style))
	r.Set(rect.X, rect.Y+rect.H-1, styled("└", w.style))
	r.Set(rect.X+rect.W-1, rect.Y+rect.H-1, styled("┘", w.style))
	if w.title != "" && rect.W > 4 {
		title := " " + text.Truncate(w.title, rect.W-4, text.Ellipsis) + " "
		drawText(r, rect.X+1, rect.Y, title, w.style)
	}
}

// Handle forwards events to the child when one is present.
func (w *Modal) Handle(ev tui.Event) bool {
	if w == nil || w.child == nil {
		return false
	}
	return w.child.Handle(ev)
}

// HandleBubble avoids forwarding a bubbled event back into the modal child.
func (w *Modal) HandleBubble(tui.Event) bool { return false }

// DragStart starts a modal drag when x,y hit the title bar.
func (w *Modal) DragStart(x, y int) (DragOp, bool) {
	if w == nil {
		return nil, false
	}
	rect := w.last
	if rect.W == 0 || rect.H == 0 {
		rect = screen.Rect{X: w.x, Y: w.y, W: w.w, H: w.h}
	}
	if y != rect.Y || x < rect.X || x >= rect.X+rect.W {
		return nil, false
	}
	return &modalDrag{modal: w, startX: rect.X, startY: rect.Y}, true
}

// ResizeStart starts a resize from the bottom-right corner. Modal owns size
// clamping, so callers only need to expose this component to pointer routing.
func (w *Modal) ResizeStart(x, y int) (DragOp, bool) {
	if w == nil {
		return nil, false
	}
	rect := w.last
	if rect.W == 0 || rect.H == 0 {
		rect = screen.Rect{X: w.x, Y: w.y, W: w.w, H: w.h}
	}
	if x != rect.X+rect.W-1 || y != rect.Y+rect.H-1 {
		return nil, false
	}
	return &modalResize{modal: w, width: w.w, height: w.h}, true
}

func (w *Modal) rebuild() {
	w.node = layout.Node{
		Dir:     layout.Column,
		Grow:    1,
		Padding: layout.Insets{Top: 1, Right: 1, Bottom: 1, Left: 1},
	}
	if w.child != nil {
		w.node.Children = []*layout.Node{w.child.Layout()}
	}
}

type modalDrag struct {
	modal          *Modal
	startX, startY int
}

type modalResize struct {
	modal         *Modal
	width, height int
}

func (op *modalResize) DragMove(dx, dy int) {
	if op == nil || op.modal == nil {
		return
	}
	op.modal.SetSize(op.width+dx, op.height+dy)
}

func (op *modalResize) DragEnd(commit bool) {
	if op == nil || op.modal == nil || commit {
		return
	}
	op.modal.SetSize(op.width, op.height)
}

func (op *modalDrag) DragMove(dx, dy int) {
	if op == nil || op.modal == nil {
		return
	}
	op.modal.SetPosition(op.startX+dx, op.startY+dy)
}

func (op *modalDrag) DragEnd(commit bool) {
	if op == nil || op.modal == nil || commit {
		return
	}
	op.modal.SetPosition(op.startX, op.startY)
}
