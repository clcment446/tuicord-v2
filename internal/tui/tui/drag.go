package tui

import "awesomeProject/internal/tui/input"

// Draggable is implemented by widgets that can start a pointer drag.
type Draggable interface {
	// DragStart receives widget-local coordinates and returns a drag operation
	// when the point is on a draggable handle.
	DragStart(x, y int) (DragOp, bool)
}

// DragOp receives pointer movement while a drag is captured.
type DragOp interface {
	// DragMove receives the pointer delta from the press that started the drag.
	DragMove(dx, dy int)
	// DragEnd finishes the drag. commit is false when the operation is canceled.
	DragEnd(commit bool)
}

// DragState describes an active drag capture.
type DragState struct {
	// Widget is the draggable widget that started the operation.
	Widget Widget
	// StartX is the x coordinate where the drag started, in root coordinates.
	StartX int
	// StartY is the y coordinate where the drag started, in root coordinates.
	StartY int
	// LastX is the most recent pointer x coordinate, in root coordinates.
	LastX int
	// LastY is the most recent pointer y coordinate, in root coordinates.
	LastY int
	// Button is the button that started the drag.
	Button input.Button
}

// DragManager owns pointer capture for a single active drag operation.
type DragManager struct {
	op     DragOp
	active DragState
}

// Active reports whether a drag currently has pointer capture.
func (d *DragManager) Active() bool {
	return d != nil && d.op != nil
}

// State returns the active drag state.
func (d *DragManager) State() (DragState, bool) {
	if !d.Active() {
		return DragState{}, false
	}
	return d.active, true
}

// HandleMouse advances the drag state machine for a mouse event.
//
// Press events hit-test for the deepest Draggable. Once a drag starts, all
// motion and release events are consumed until the drag commits or cancels.
func (d *DragManager) HandleMouse(ev input.MouseEvent, hits HitIndex) bool {
	if d == nil {
		return false
	}
	if d.op != nil {
		switch ev.Kind {
		case input.MouseMotion:
			d.active.LastX = ev.X
			d.active.LastY = ev.Y
			d.op.DragMove(ev.X-d.active.StartX, ev.Y-d.active.StartY)
			return true
		case input.MouseRelease:
			d.end(true)
			return true
		default:
			return true
		}
	}
	if ev.Kind != input.MousePress || ev.Btn != input.ButtonLeft {
		return false
	}
	path := hits.Path(ev.X, ev.Y)
	for i := len(path) - 1; i >= 0; i-- {
		draggable, ok := path[i].Widget.(Draggable)
		if !ok {
			continue
		}
		op, ok := draggable.DragStart(path[i].X, path[i].Y)
		if !ok || op == nil {
			continue
		}
		d.op = op
		d.active = DragState{
			Widget: path[i].Widget,
			StartX: ev.X,
			StartY: ev.Y,
			LastX:  ev.X,
			LastY:  ev.Y,
			Button: ev.Btn,
		}
		return true
	}
	return false
}

// HandleWidgetMouse starts pointer capture on a transient widget drawn outside
// the retained hit index. The component decides whether the press hit its drag
// or resize handle; callers never duplicate that geometry.
func (d *DragManager) HandleWidgetMouse(ev input.MouseEvent, w Widget) bool {
	// Only the left button starts a drag, matching HandleMouse. Without this a
	// right-click press on a transient widget (a plugin viewport) began dragging.
	if d == nil || w == nil || d.op != nil || ev.Kind != input.MousePress || ev.Btn != input.ButtonLeft {
		return false
	}
	var op DragOp
	var ok bool
	if draggable, draggableOK := w.(Draggable); draggableOK {
		op, ok = draggable.DragStart(ev.X, ev.Y)
	}
	if !ok {
		if resizable, resizableOK := w.(Resizable); resizableOK {
			op, ok = resizable.ResizeStart(ev.X, ev.Y)
		}
	}
	if !ok || op == nil {
		return false
	}
	d.op = op
	d.active = DragState{Widget: w, StartX: ev.X, StartY: ev.Y, LastX: ev.X, LastY: ev.Y, Button: ev.Btn}
	return true
}

// Cancel cancels the active drag, if any.
func (d *DragManager) Cancel() bool {
	if !d.Active() {
		return false
	}
	d.end(false)
	return true
}

func (d *DragManager) end(commit bool) {
	op := d.op
	d.op = nil
	d.active = DragState{}
	op.DragEnd(commit)
}
