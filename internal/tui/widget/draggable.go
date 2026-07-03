package widget

import (
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

// Draggable wraps any widget and lets the whole wrapped region be dragged.
type Draggable struct {
	child tui.Widget
	x, y  int
	w, h  int
	node  layout.Node
}

// NewDraggable returns a draggable wrapper for child.
func NewDraggable(child tui.Widget) *Draggable {
	d := &Draggable{child: child}
	d.rebuild()
	return d
}

// SetPosition sets the wrapper's top-left offset.
func (d *Draggable) SetPosition(x, y int) {
	if d == nil {
		return
	}
	d.x = maxInt(x, 0)
	d.y = maxInt(y, 0)
	d.rebuild()
}

// Child returns the wrapped widget.
func (d *Draggable) Child() tui.Widget {
	if d == nil {
		return nil
	}
	return d.child
}

// Children returns the wrapped widget for retained-tree traversal.
func (d *Draggable) Children() []tui.Widget {
	if d == nil || d.child == nil {
		return nil
	}
	return []tui.Widget{d.child}
}

// Measure returns the wrapped widget size.
func (d *Draggable) Measure(avail tui.Size) tui.Size {
	if d == nil {
		return tui.Size{}
	}
	d.w = avail.W
	d.h = avail.H
	if d.child != nil {
		return d.child.Measure(avail)
	}
	return avail
}

// Layout returns the draggable layout node.
func (d *Draggable) Layout() *layout.Node {
	if d == nil {
		return nil
	}
	return &d.node
}

// Draw intentionally does nothing; the child draws itself.
func (d *Draggable) Draw(screen.Region) {}

// Handle forwards events to the wrapped widget.
func (d *Draggable) Handle(ev tui.Event) bool {
	return d != nil && d.child != nil && d.child.Handle(ev)
}

// DragStart starts dragging from anywhere inside the wrapper.
func (d *Draggable) DragStart(x, y int) (DragOp, bool) {
	if d == nil || x < 0 || y < 0 {
		return nil, false
	}
	return &dragWrapperOp{d: d, x: d.x, y: d.y}, true
}

func (d *Draggable) rebuild() {
	d.node = layout.Node{Dir: layout.Column, Grow: 1, Padding: layout.Insets{Top: d.y, Left: d.x}}
	if d.child != nil {
		d.node.Children = []*layout.Node{d.child.Layout()}
	}
}

type dragWrapperOp struct {
	d    *Draggable
	x, y int
}

func (op *dragWrapperOp) DragMove(dx, dy int) {
	if op == nil || op.d == nil {
		return
	}
	op.d.SetPosition(op.x+dx, op.y+dy)
}

func (op *dragWrapperOp) DragEnd(commit bool) {
	if op == nil || op.d == nil || commit {
		return
	}
	op.d.SetPosition(op.x, op.y)
}
