package widget

import (
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

// DragOp is an active pointer drag operation.
type DragOp = tui.DragOp

// Split arranges two child widgets with a draggable divider.
type Split struct {
	first  tui.Widget
	second tui.Widget
	dir    layout.Direction
	basis  int
	min    int
	max    int
	main   int
	style  screen.Style
	node   layout.Node
}

// NewSplit returns a row split with first on the left and second on the right.
func NewSplit(first, second tui.Widget) *Split {
	s := &Split{first: first, second: second, dir: layout.Row, basis: 24, min: 1}
	s.rebuild()
	return s
}

// First returns the first child widget.
func (w *Split) First() tui.Widget {
	if w == nil {
		return nil
	}
	return w.first
}

// Second returns the second child widget.
func (w *Split) Second() tui.Widget {
	if w == nil {
		return nil
	}
	return w.second
}

// Children returns the split panes for retained-tree traversal.
func (w *Split) Children() []tui.Widget {
	if w == nil {
		return nil
	}
	children := make([]tui.Widget, 0, 2)
	if w.first != nil {
		children = append(children, w.first)
	}
	if w.second != nil {
		children = append(children, w.second)
	}
	return children
}

// Basis sets the first pane's preferred size in cells and returns w.
func (w *Split) Basis(cells int) *Split {
	if w == nil {
		return nil
	}
	w.basis = w.clampBasis(cells)
	w.rebuild()
	return w
}

// MinFirst sets the first pane's minimum size and returns w.
func (w *Split) MinFirst(cells int) *Split {
	if w == nil {
		return nil
	}
	w.min = maxInt(cells, 0)
	w.basis = w.clampBasis(w.basis)
	w.rebuild()
	return w
}

// MaxFirst sets the first pane's maximum size and returns w.
func (w *Split) MaxFirst(cells int) *Split {
	if w == nil {
		return nil
	}
	w.max = maxInt(cells, 0)
	w.basis = w.clampBasis(w.basis)
	w.rebuild()
	return w
}

// Vertical arranges panes left to right and returns w.
func (w *Split) Vertical() *Split {
	if w == nil {
		return nil
	}
	w.dir = layout.Row
	w.rebuild()
	return w
}

// Horizontal arranges panes top to bottom and returns w.
func (w *Split) Horizontal() *Split {
	if w == nil {
		return nil
	}
	w.dir = layout.Column
	w.rebuild()
	return w
}

// SetStyle sets the style used for the divider.
func (w *Split) SetStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.style = style
}

// CanFocus reports that the divider can receive keyboard focus.
func (w *Split) CanFocus() bool {
	return w != nil
}

// Measure returns a best-effort preferred size for both panes and divider.
func (w *Split) Measure(avail tui.Size) tui.Size {
	if w == nil {
		return tui.Size{}
	}
	if w.dir == layout.Column {
		w.main = avail.H
	} else {
		w.main = avail.W
	}
	first := tui.Size{}
	second := tui.Size{}
	if w.first != nil {
		first = w.first.Measure(avail)
	}
	if w.second != nil {
		second = w.second.Measure(avail)
	}
	if w.dir == layout.Column {
		return tui.Size{W: maxInt(first.W, second.W), H: first.H + 1 + second.H}
	}
	return tui.Size{W: first.W + 1 + second.W, H: maxInt(first.H, second.H)}
}

// Layout returns the split node with a one-cell gap for the divider.
func (w *Split) Layout() *layout.Node {
	if w == nil {
		return nil
	}
	return &w.node
}

// Draw renders the divider into r.
func (w *Split) Draw(r screen.Region) {
	if w == nil {
		return
	}
	if w.dir == layout.Column {
		w.main = r.Height()
		y := w.clampDivider(r.Height())
		if y >= 0 && y < r.Height() {
			for x := 0; x < r.Width(); x++ {
				r.Set(x, y, styled("─", w.style))
			}
		}
		return
	}
	w.main = r.Width()
	x := w.clampDivider(r.Width())
	if x >= 0 && x < r.Width() {
		for y := 0; y < r.Height(); y++ {
			r.Set(x, y, styled("│", w.style))
		}
	}
}

// Handle adjusts the divider for Alt+arrow keyboard events.
func (w *Split) Handle(ev tui.Event) bool {
	if w == nil {
		return false
	}
	key, ok := ev.(input.KeyEvent)
	if !ok || key.Release || key.Mods&input.Alt == 0 {
		return false
	}
	switch key.Key {
	case input.KeyLeft:
		if w.dir == layout.Row {
			w.Basis(w.basis - 1)
			return true
		}
	case input.KeyRight:
		if w.dir == layout.Row {
			w.Basis(w.basis + 1)
			return true
		}
	case input.KeyUp:
		if w.dir == layout.Column {
			w.Basis(w.basis - 1)
			return true
		}
	case input.KeyDown:
		if w.dir == layout.Column {
			w.Basis(w.basis + 1)
			return true
		}
	}
	return false
}

// DragStart starts a divider drag when x,y hit the divider.
func (w *Split) DragStart(x, y int) (DragOp, bool) {
	if w == nil {
		return nil, false
	}
	if w.dir == layout.Column {
		if y != w.dragDivider() {
			return nil, false
		}
		return &splitDrag{split: w, start: w.basis}, true
	}
	if x != w.dragDivider() {
		return nil, false
	}
	return &splitDrag{split: w, start: w.basis}, true
}

func (w *Split) rebuild() {
	first := &layout.Node{Basis: w.clampBasis(w.basis), Min: w.min, Max: w.max}
	second := &layout.Node{Grow: 1, Min: 1}
	if w.first != nil {
		first.Children = []*layout.Node{stretchNode(w.first.Layout())}
	}
	if w.second != nil {
		second.Children = []*layout.Node{stretchNode(w.second.Layout())}
	}
	w.node = layout.Node{Dir: w.dir, Grow: 1, Gap: 1, Children: []*layout.Node{first, second}}
}

func stretchNode(node *layout.Node) *layout.Node {
	if node != nil && node.Grow == 0 {
		node.Grow = 1
	}
	return node
}

func (w *Split) clampBasis(v int) int {
	return clampInt(v, w.min, w.max)
}

func (w *Split) clampDivider(size int) int {
	return clampInt(w.basis, 0, maxInt(size-1, 0))
}

func (w *Split) dragDivider() int {
	if w.main > 0 {
		return w.clampDivider(w.main)
	}
	return w.clampBasis(w.basis)
}

type splitDrag struct {
	split *Split
	start int
}

func (op *splitDrag) DragMove(dx, dy int) {
	if op == nil || op.split == nil {
		return
	}
	delta := dx
	if op.split.dir == layout.Column {
		delta = dy
	}
	op.split.Basis(op.start + delta)
}

func (op *splitDrag) DragEnd(commit bool) {
	if op == nil || op.split == nil || commit {
		return
	}
	op.split.Basis(op.start)
}
