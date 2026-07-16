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
	first     tui.Widget
	second    tui.Widget
	dir       layout.Direction
	basis     int
	min       int
	max       int
	minSecond int
	maxSecond int
	main      int
	style     screen.Style
	focused   bool
	focusable bool

	collapsibleFirst  bool
	collapsibleSecond bool
	collapsedFirst    bool
	collapsedSecond   bool
	expandedBasis     int

	node layout.Node
}

// NewSplit returns a row split with first on the left and second on the right.
func NewSplit(first, second tui.Widget) *Split {
	s := &Split{first: first, second: second, dir: layout.Row, basis: 24, min: 1, minSecond: 1}
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
	if w.first != nil && !w.collapsedFirst {
		children = append(children, w.first)
	}
	if w.second != nil && !w.collapsedSecond {
		children = append(children, w.second)
	}
	return children
}

// Basis sets the first pane's preferred size in cells and returns w.
func (w *Split) Basis(cells int) *Split {
	if w == nil {
		return nil
	}
	if w.collapsibleFirst && cells <= 0 {
		w.collapseFirst()
		return w
	}
	if w.collapsibleSecond && w.main > 0 && cells >= w.main-1 {
		w.collapseSecond()
		return w
	}
	w.collapsedFirst = false
	w.collapsedSecond = false
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

// MinSecond sets the second pane's minimum size and returns w.
func (w *Split) MinSecond(cells int) *Split {
	if w == nil {
		return nil
	}
	w.minSecond = maxInt(cells, 0)
	w.rebuild()
	return w
}

// MaxSecond sets the second pane's maximum size and returns w.
func (w *Split) MaxSecond(cells int) *Split {
	if w == nil {
		return nil
	}
	w.maxSecond = maxInt(cells, 0)
	w.rebuild()
	return w
}

// CollapsibleFirst lets the first pane collapse into a one-cell toggle strip.
func (w *Split) CollapsibleFirst() *Split {
	if w == nil {
		return nil
	}
	w.collapsibleFirst = true
	w.rebuild()
	return w
}

// CollapsibleSecond lets the second pane collapse into a one-cell toggle strip.
func (w *Split) CollapsibleSecond() *Split {
	if w == nil {
		return nil
	}
	w.collapsibleSecond = true
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

// CanFocus reports whether the split itself belongs in keyboard focus order.
func (w *Split) CanFocus() bool {
	// A split's divider and collapsed toggle are layout affordances, not
	// interactive keyboard elements. They remain mouse-operable through Handle.
	return w != nil && w.focusable
}

// SetFocusEnabled controls whether the split selector enters keyboard focus
// order. Mouse interaction remains available independently of this setting.
func (w *Split) SetFocusEnabled(enabled bool) {
	if w != nil {
		w.focusable = enabled
	}
}

// SetFocusOwner records whether the split selector itself owns keyboard focus.
func (w *Split) SetFocusOwner(focused bool) {
	if w != nil {
		w.focused = focused
	}
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
		if w.collapsedFirst || w.collapsedSecond {
			w.drawCollapsed(r)
			return
		}
		y := w.clampDivider(r.Height())
		if y >= 0 && y < r.Height() {
			for x := 0; x < r.Width(); x++ {
				r.Set(x, y, styled("─", w.dividerStyle()))
			}
		}
		return
	}
	w.main = r.Width()
	if w.collapsedFirst || w.collapsedSecond {
		w.drawCollapsed(r)
		return
	}
	x := w.clampDivider(r.Width())
	if x >= 0 && x < r.Width() {
		for y := 0; y < r.Height(); y++ {
			r.Set(x, y, styled("│", w.dividerStyle()))
		}
	}
}

// Handle adjusts the divider for Alt+arrow keyboard events, then offers
// unhandled events to panes from front to back.
func (w *Split) Handle(ev tui.Event) bool {
	if w == nil {
		return false
	}
	if mouse, ok := ev.(input.MouseEvent); ok {
		if mouse.Kind == input.MousePress && mouse.Btn == input.ButtonLeft && w.hitCollapsedToggle(mouse.X, mouse.Y) {
			w.expand()
			return true
		}
		return false
	}
	key, ok := ev.(input.KeyEvent)
	if ok && !key.Release && w.focused {
		switch key.Key {
		case input.KeyEnter:
			if w.collapsedFirst || w.collapsedSecond {
				w.expand()
			}
			return true
		case input.KeyRune:
			if key.Rune == ' ' {
				if w.collapsedFirst || w.collapsedSecond {
					w.expand()
				} else if w.collapsibleFirst {
					w.collapseFirst()
				} else if w.collapsibleSecond {
					w.collapseSecond()
				}
				return true
			}
		}
	}
	if ok && !key.Release && key.Mods&input.Alt != 0 {
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
	}
	if w.second != nil && w.second.Handle(ev) {
		return true
	}
	if w.first != nil && w.first.Handle(ev) {
		return true
	}
	return false
}

// DragStart starts a divider drag when x,y hit the divider.
func (w *Split) DragStart(x, y int) (DragOp, bool) {
	if w == nil {
		return nil, false
	}
	if w.collapsedFirst || w.collapsedSecond {
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
	if w.expandedBasis == 0 {
		w.expandedBasis = w.basis
	}
	if w.collapsedFirst {
		first := &layout.Node{Basis: 1, Min: 1, Max: 1}
		second := &layout.Node{Grow: 1, Min: w.minSecond, Max: w.maxSecond}
		if w.second != nil {
			second.Children = []*layout.Node{stretchNode(w.second.Layout())}
		}
		w.node = layout.Node{Dir: w.dir, Grow: 1, Gap: 0, Children: []*layout.Node{first, second}}
		return
	}
	if w.collapsedSecond {
		first := &layout.Node{Grow: 1, Min: 1}
		second := &layout.Node{Basis: 1, Min: 1, Max: 1}
		if w.first != nil {
			first.Children = []*layout.Node{stretchNode(w.first.Layout())}
		}
		w.node = layout.Node{Dir: w.dir, Grow: 1, Gap: 0, Children: []*layout.Node{first, second}}
		return
	}
	first := &layout.Node{Basis: w.clampBasis(w.basis), Min: w.min, Max: w.max}
	second := &layout.Node{Grow: 1, Min: w.minSecond, Max: w.maxSecond}
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

func (w *Split) collapseFirst() {
	if w == nil || w.collapsedFirst {
		return
	}
	w.expandedBasis = maxInt(w.basis, 1)
	w.collapsedFirst = true
	w.collapsedSecond = false
	w.rebuild()
}

func (w *Split) collapseSecond() {
	if w == nil || w.collapsedSecond {
		return
	}
	w.expandedBasis = maxInt(w.basis, 1)
	w.collapsedSecond = true
	w.collapsedFirst = false
	w.rebuild()
}

func (w *Split) expand() {
	if w == nil || (!w.collapsedFirst && !w.collapsedSecond) {
		return
	}
	basis := w.expandedBasis
	w.collapsedFirst = false
	w.collapsedSecond = false
	if basis <= 0 {
		basis = w.basis
	}
	w.basis = w.clampBasis(basis)
	w.rebuild()
}

func (w *Split) hitCollapsedToggle(x, y int) bool {
	if w == nil {
		return false
	}
	switch {
	case w.dir == layout.Column && w.collapsedFirst:
		return y == 0
	case w.dir == layout.Column && w.collapsedSecond:
		return w.main <= 0 || y == w.main-1
	case w.collapsedFirst:
		return x == 0
	case w.collapsedSecond:
		return w.main <= 0 || x == w.main-1
	default:
		return false
	}
}

func (w *Split) drawCollapsed(r screen.Region) {
	if w.dir == layout.Column {
		y := 0
		tri := "▼"
		if w.collapsedSecond {
			y = maxInt(r.Height()-1, 0)
			tri = "▲"
		}
		for x := 0; x < r.Width(); x++ {
			r.Set(x, y, styled("─", w.dividerStyle()))
		}
		if r.Width() > 0 && r.Height() > 0 {
			r.Set(r.Width()/2, y, styled(tri, w.dividerStyle()))
		}
		return
	}
	x := 0
	tri := "▶"
	if w.collapsedSecond {
		x = maxInt(r.Width()-1, 0)
		tri = "◀"
	}
	for y := 0; y < r.Height(); y++ {
		r.Set(x, y, styled("│", w.dividerStyle()))
	}
	if r.Width() > 0 && r.Height() > 0 {
		r.Set(x, r.Height()/2, styled(tri, w.dividerStyle()))
	}
}

func (w *Split) dividerStyle() screen.Style {
	if w != nil && w.focused {
		style := w.style
		style.Attrs |= screen.Bold
		return style
	}
	return w.style
}

type splitDrag struct {
	split *Split
	start int
	moved bool
}

func (op *splitDrag) DragMove(dx, dy int) {
	if op == nil || op.split == nil {
		return
	}
	delta := dx
	if op.split.dir == layout.Column {
		delta = dy
	}
	if delta != 0 {
		op.moved = true
	}
	op.split.Basis(op.start + delta)
}

func (op *splitDrag) DragEnd(commit bool) {
	if op == nil || op.split == nil {
		return
	}
	if !commit {
		op.split.Basis(op.start)
		return
	}
	// A press-and-release on the divider with no movement is a click, not a
	// drag: toggle-collapse the adjacent pane, remembering its size so a second
	// click restores it.
	if !op.moved {
		op.split.toggleCollapse()
	}
}

// toggleCollapse collapses the first collapsible pane on a divider click, or
// restores a collapsed pane to its previous size. Panes that were not marked
// collapsible are left untouched.
func (w *Split) toggleCollapse() {
	if w == nil {
		return
	}
	if w.collapsedFirst || w.collapsedSecond {
		w.expand()
		return
	}
	switch {
	case w.collapsibleFirst:
		w.collapseFirst()
	case w.collapsibleSecond:
		w.collapseSecond()
	}
}
