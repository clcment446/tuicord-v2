package widget

import (
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

// Button is a one-line focusable command widget.
type Button struct {
	label string
	style screen.Style
	press func()
	node  layout.Node
}

// NewButton returns a button with label and optional press callback.
func NewButton(label string, press func()) *Button {
	return &Button{label: label, press: press, node: layout.Node{Basis: 1, Min: 1}}
}

// Label returns the button label.
func (b *Button) Label() string {
	if b == nil {
		return ""
	}
	return b.label
}

// SetLabel replaces the button label.
func (b *Button) SetLabel(label string) {
	if b == nil {
		return
	}
	b.label = label
}

// SetStyle sets the button style.
func (b *Button) SetStyle(style screen.Style) {
	if b == nil {
		return
	}
	b.style = style
}

// CanFocus reports that the button can receive keyboard focus.
func (b *Button) CanFocus() bool {
	return b != nil
}

// Measure returns the label size.
func (b *Button) Measure(avail tui.Size) tui.Size {
	if b == nil {
		return tui.Size{}
	}
	width := text.Width(b.label)
	if avail.W > 0 {
		width = minInt(width, avail.W)
	}
	return tui.Size{W: width, H: 1}
}

// Layout returns the button layout node.
func (b *Button) Layout() *layout.Node {
	if b == nil {
		return nil
	}
	return &b.node
}

// Draw renders the button label.
func (b *Button) Draw(r screen.Region) {
	if b == nil {
		return
	}
	drawPaddedText(r, 0, b.label, b.style)
}

// Handle runs the callback on Enter, Space, or left mouse press.
func (b *Button) Handle(ev tui.Event) bool {
	if b == nil {
		return false
	}
	switch ev := ev.(type) {
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		if ev.Key == input.KeyEnter || ev.Key == input.KeyRune && ev.Rune == ' ' {
			b.click()
			return true
		}
	case input.MouseEvent:
		if ev.Kind == input.MousePress && ev.Btn == input.ButtonLeft {
			b.click()
			return true
		}
	}
	return false
}

func (b *Button) click() {
	if b.press != nil {
		b.press()
	}
}
