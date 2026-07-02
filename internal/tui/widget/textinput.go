package widget

import (
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	tuitext "awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

// TextInput is a single-line editable text field.
type TextInput struct {
	value            string
	placeholder      string
	cursor           int
	scroll           int
	focused          bool
	style            screen.Style
	placeholderStyle screen.Style
	cursorStyle      screen.Style
	node             layout.Node
}

// NewTextInput returns an empty text input with placeholder text.
func NewTextInput(placeholder string) *TextInput {
	return &TextInput{
		placeholder:      placeholder,
		focused:          true,
		placeholderStyle: screen.Style{Attrs: screen.Dim},
		cursorStyle:      screen.Style{Attrs: screen.Reverse},
		node:             layout.Node{Basis: 1, Min: 1},
	}
}

// Value returns the current input value.
func (w *TextInput) Value() string {
	if w == nil {
		return ""
	}
	return w.value
}

// SetValue replaces the current input value and moves the cursor to the end.
func (w *TextInput) SetValue(value string) {
	if w == nil {
		return
	}
	w.value = value
	w.cursor = len(value)
	w.scroll = 0
}

// Cursor returns the cursor byte offset, always on a grapheme boundary.
func (w *TextInput) Cursor() int {
	if w == nil {
		return 0
	}
	return w.cursor
}

// SetCursor moves the cursor to the nearest preceding grapheme boundary.
func (w *TextInput) SetCursor(offset int) {
	if w == nil {
		return
	}
	w.cursor = tuitext.PrevBoundary(w.value, tuitext.NextBoundary(w.value, offset))
}

// SetFocused controls whether Draw renders the cursor.
func (w *TextInput) SetFocused(focused bool) {
	if w == nil {
		return
	}
	w.focused = focused
}

// CanFocus reports that the input can receive keyboard focus.
func (w *TextInput) CanFocus() bool {
	return w != nil
}

// SetStyle sets the style used for input text.
func (w *TextInput) SetStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.style = style
}

// SetPlaceholderStyle sets the style used when the input is empty.
func (w *TextInput) SetPlaceholderStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.placeholderStyle = style
}

// Measure returns a one-line preferred size.
func (w *TextInput) Measure(avail tui.Size) tui.Size {
	width := 1
	if w != nil {
		width = maxInt(tuitext.Width(w.value), tuitext.Width(w.placeholder))
	}
	if avail.W > 0 {
		width = minInt(width, avail.W)
	}
	return tui.Size{W: maxInt(width, 1), H: 1}
}

// Layout returns the layout node for this text input.
func (w *TextInput) Layout() *layout.Node {
	if w == nil {
		return nil
	}
	return &w.node
}

// Draw renders the input value, placeholder, and cursor.
func (w *TextInput) Draw(r screen.Region) {
	if w == nil || r.Height() == 0 {
		return
	}
	clearLine(r, 0, w.style)
	if w.value == "" {
		drawText(r, 0, 0, tuitext.Truncate(w.placeholder, r.Width(), tuitext.Ellipsis), w.placeholderStyle)
		if w.focused && r.Width() > 0 {
			r.Set(0, 0, styled(" ", w.cursorStyle))
		}
		return
	}

	w.ensureCursorVisible(r.Width())
	visible := visibleFromCell(w.value, w.scroll, r.Width())
	drawText(r, 0, 0, visible, w.style)
	if !w.focused || r.Width() <= 0 {
		return
	}
	cursorX := cellOffsetOfByte(w.value, w.cursor) - w.scroll
	if cursorX < 0 || cursorX >= r.Width() {
		return
	}
	content := " "
	for cluster := range tuitext.Clusters(w.value[w.cursor:]) {
		if cluster.Width > 0 {
			content = cluster.Text
		}
		break
	}
	r.Set(cursorX, 0, styled(content, w.cursorStyle))
}

// Handle edits the input for key and paste events.
func (w *TextInput) Handle(ev tui.Event) bool {
	if w == nil {
		return false
	}
	switch ev := ev.(type) {
	case input.PasteEvent:
		w.insert(ev.Text)
		return ev.Text != ""
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		switch ev.Key {
		case input.KeyRune:
			w.insert(string(ev.Rune))
			return true
		case input.KeyLeft:
			w.cursor = tuitext.PrevBoundary(w.value, w.cursor)
			return true
		case input.KeyRight:
			w.cursor = tuitext.NextBoundary(w.value, w.cursor)
			return true
		case input.KeyHome:
			w.cursor = 0
			return true
		case input.KeyEnd:
			w.cursor = len(w.value)
			return true
		case input.KeyBackspace:
			prev := tuitext.PrevBoundary(w.value, w.cursor)
			if prev != w.cursor {
				w.value = w.value[:prev] + w.value[w.cursor:]
				w.cursor = prev
			}
			return true
		case input.KeyDelete:
			next := tuitext.NextBoundary(w.value, w.cursor)
			if next != w.cursor {
				w.value = w.value[:w.cursor] + w.value[next:]
			}
			return true
		}
	}
	return false
}

func (w *TextInput) insert(s string) {
	w.value = w.value[:w.cursor] + s + w.value[w.cursor:]
	w.cursor += len(s)
}

func (w *TextInput) ensureCursorVisible(width int) {
	if width <= 0 {
		w.scroll = 0
		return
	}
	cursorCell := cellOffsetOfByte(w.value, w.cursor)
	if cursorCell < w.scroll {
		w.scroll = cursorCell
	}
	if cursorCell >= w.scroll+width {
		w.scroll = cursorCell - width + 1
	}
	if w.scroll < 0 {
		w.scroll = 0
	}
}
