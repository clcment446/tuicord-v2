package widget

import (
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

// Text draws static plain text.
type Text struct {
	content string
	style   screen.Style
	wrap    bool
	node    layout.Node
}

// NewText returns a text widget containing content.
func NewText(content string) *Text {
	return &Text{content: content, wrap: true, node: layout.Node{Grow: 1}}
}

// Content returns the widget text.
func (w *Text) Content() string {
	if w == nil {
		return ""
	}
	return w.content
}

// SetContent replaces the widget text.
func (w *Text) SetContent(content string) {
	if w == nil {
		return
	}
	w.content = content
}

// SetStyle sets the style used for every drawn cell.
func (w *Text) SetStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.style = style
}

// SetWrap controls whether text wraps to the available width.
func (w *Text) SetWrap(wrap bool) {
	if w == nil {
		return
	}
	w.wrap = wrap
}

// Measure returns the natural size of the text within avail.
func (w *Text) Measure(avail tui.Size) tui.Size {
	if w == nil || w.content == "" {
		return tui.Size{}
	}
	width := avail.W
	if width <= 0 {
		width = text.Width(w.content)
	}
	lines := []string{w.content}
	if w.wrap {
		lines = text.Wrap(w.content, width)
	}
	measuredW := 0
	for _, line := range lines {
		measuredW = maxInt(measuredW, text.Width(line))
	}
	if avail.W > 0 {
		measuredW = minInt(measuredW, avail.W)
	}
	return tui.Size{W: measuredW, H: len(lines)}
}

// Layout returns the layout node for this text widget.
func (w *Text) Layout() *layout.Node {
	if w == nil {
		return nil
	}
	return &w.node
}

// Draw renders the text into r.
func (w *Text) Draw(r screen.Region) {
	if w == nil {
		return
	}
	clear(r, w.style)
	lines := []string{w.content}
	if w.wrap {
		lines = text.Wrap(w.content, r.Width())
	}
	for y, line := range lines {
		if y >= r.Height() {
			break
		}
		drawPaddedText(r, y, line, w.style)
	}
}

// Handle ignores input events and reports them unconsumed.
func (w *Text) Handle(tui.Event) bool {
	return false
}
