package widget

import (
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

// BorderChars defines the glyphs used to draw a border.
type BorderChars struct {
	// TopLeft is the upper-left corner.
	TopLeft string
	// TopRight is the upper-right corner.
	TopRight string
	// BottomLeft is the lower-left corner.
	BottomLeft string
	// BottomRight is the lower-right corner.
	BottomRight string
	// Horizontal is the top and bottom edge.
	Horizontal string
	// Vertical is the left and right edge.
	Vertical string
}

// RoundedBorder is the default single-cell border style.
var RoundedBorder = BorderChars{
	TopLeft: "┌", TopRight: "┐", BottomLeft: "└", BottomRight: "┘",
	Horizontal: "─", Vertical: "│",
}

// Border draws a frame and exposes a padded child layout.
type Border struct {
	child      tui.Widget
	title      string
	chars      BorderChars
	style      screen.Style
	focusStyle screen.Style
	focused    bool
	node       layout.Node
}

// NewBorder returns a border around child.
func NewBorder(child tui.Widget) *Border {
	b := &Border{child: child, chars: RoundedBorder}
	b.rebuild()
	return b
}

// Child returns the contained widget.
func (w *Border) Child() tui.Widget {
	if w == nil {
		return nil
	}
	return w.child
}

// Children returns the contained widget for retained-tree traversal.
func (w *Border) Children() []tui.Widget {
	if w == nil || w.child == nil {
		return nil
	}
	return []tui.Widget{w.child}
}

// SetChild replaces the contained widget.
func (w *Border) SetChild(child tui.Widget) {
	if w == nil {
		return
	}
	w.child = child
	w.rebuild()
}

// Title returns the border title.
func (w *Border) Title() string {
	if w == nil {
		return ""
	}
	return w.title
}

// SetTitle replaces the border title.
func (w *Border) SetTitle(title string) {
	if w == nil {
		return
	}
	w.title = title
}

// SetStyle sets the style used for the border cells.
func (w *Border) SetStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.style = style
}

// SetFocusStyle sets the border style while focus is inside the border.
func (w *Border) SetFocusStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.focusStyle = style
}

// SetFocused records whether keyboard focus is inside this border.
func (w *Border) SetFocused(focused bool) {
	if w == nil {
		return
	}
	w.focused = focused
}

// SetChars replaces the border glyphs.
func (w *Border) SetChars(chars BorderChars) {
	if w == nil {
		return
	}
	w.chars = chars
}

// Measure returns the child's preferred size plus a one-cell frame.
func (w *Border) Measure(avail tui.Size) tui.Size {
	if w == nil {
		return tui.Size{}
	}
	inner := tui.Size{W: maxInt(avail.W-2, 0), H: maxInt(avail.H-2, 0)}
	size := inner
	if w.child != nil {
		size = w.child.Measure(inner)
	}
	return tui.Size{W: size.W + 2, H: size.H + 2}
}

// Layout returns the frame node with the child inset by one cell.
func (w *Border) Layout() *layout.Node {
	if w == nil {
		return nil
	}
	return &w.node
}

// Draw renders the border frame and title into r.
func (w *Border) Draw(r screen.Region) {
	if w == nil || r.Width() <= 0 || r.Height() <= 0 {
		return
	}
	w.drawFrame(r)
}

// Handle forwards events to the child when one is present.
func (w *Border) Handle(ev tui.Event) bool {
	if w == nil || w.child == nil {
		return false
	}
	if _, ok := ev.(input.MouseEvent); ok {
		return false
	}
	return w.child.Handle(ev)
}

// HandleBubble keeps a border transparent while the runtime walks the exact
// focused ancestry. Calling Handle here would redispatch into its child.
func (w *Border) HandleBubble(tui.Event) bool { return false }

func (w *Border) rebuild() {
	w.node = layout.Node{
		Dir:     layout.Column,
		Grow:    1,
		Padding: layout.Insets{Top: 1, Right: 1, Bottom: 1, Left: 1},
	}
	if w.child != nil {
		w.node.Children = []*layout.Node{w.child.Layout()}
	}
}

func (w *Border) drawFrame(r screen.Region) {
	style := w.style
	if w.focused && w.focusStyle != (screen.Style{}) {
		style = w.focusStyle
	}
	lastX := r.Width() - 1
	lastY := r.Height() - 1
	for x := 0; x < r.Width(); x++ {
		r.Set(x, 0, styled(w.chars.Horizontal, style))
		if lastY > 0 {
			r.Set(x, lastY, styled(w.chars.Horizontal, style))
		}
	}
	if lastX > 0 {
		for y := 0; y < r.Height(); y++ {
			r.Set(0, y, styled(w.chars.Vertical, style))
			r.Set(lastX, y, styled(w.chars.Vertical, style))
		}
	}
	r.Set(0, 0, styled(w.chars.TopLeft, style))
	if lastX > 0 {
		r.Set(lastX, 0, styled(w.chars.TopRight, style))
	}
	if lastY > 0 {
		r.Set(0, lastY, styled(w.chars.BottomLeft, style))
		if lastX > 0 {
			r.Set(lastX, lastY, styled(w.chars.BottomRight, style))
		}
	}
	if w.title == "" || r.Width() <= 4 {
		return
	}
	title := " " + text.Truncate(w.title, r.Width()-4, text.Ellipsis) + " "
	drawText(r, 1, 0, title, style)
}
