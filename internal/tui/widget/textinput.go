package widget

import (
	"unicode"

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
	readOnly         bool
	cursorVisible    bool
	style            screen.Style
	placeholderStyle screen.Style
	cursorStyle      screen.Style
	onSubmit         func(string)
	onChange         func(string)
	node             layout.Node
}

// NewTextInput returns an empty text input with placeholder text.
func NewTextInput(placeholder string) *TextInput {
	return &TextInput{
		placeholder:      placeholder,
		focused:          true,
		cursorVisible:    true,
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
	w.showCursor()
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
	w.showCursor()
}

// SetFocused controls whether Draw renders the cursor.
func (w *TextInput) SetFocused(focused bool) {
	if w == nil {
		return
	}
	w.focused = focused
	if focused {
		w.showCursor()
	}
}

// CanFocus reports that the input can receive keyboard focus. A read-only input
// declines focus so Tab navigation skips it.
func (w *TextInput) CanFocus() bool {
	return w != nil && !w.readOnly
}

// SetReadOnly toggles read-only mode: the input ignores editing and submission
// events and no longer accepts focus. It is used for the composer in channels
// where the account lacks SEND_MESSAGES (rules, most announcement channels).
func (w *TextInput) SetReadOnly(readOnly bool) {
	if w == nil {
		return
	}
	w.readOnly = readOnly
	if readOnly {
		w.focused = false
	}
}

// ReadOnly reports whether the input is in read-only mode.
func (w *TextInput) ReadOnly() bool {
	return w != nil && w.readOnly
}

// PreferredFocus reports that text inputs should receive initial focus.
func (w *TextInput) PreferredFocus() bool {
	return w != nil
}

// SetStyle sets the style used for input text.
func (w *TextInput) SetStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.style = style
}

// SetPlaceholder replaces the placeholder text shown when the input is empty.
func (w *TextInput) SetPlaceholder(placeholder string) {
	if w == nil {
		return
	}
	w.placeholder = placeholder
}

// SetPlaceholderStyle sets the style used when the input is empty.
func (w *TextInput) SetPlaceholderStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.placeholderStyle = style
}

// SetCursorStyle sets the style used for the focused cursor cell.
func (w *TextInput) SetCursorStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.cursorStyle = style
}

// OnSubmit registers a callback invoked with the current value when the user
// presses Enter. Passing nil clears the callback.
func (w *TextInput) OnSubmit(fn func(string)) {
	if w == nil {
		return
	}
	w.onSubmit = fn
}

// OnChange registers a callback invoked with the current value whenever the
// text changes (typing, deleting, pasting). Passing nil clears the callback.
func (w *TextInput) OnChange(fn func(string)) {
	if w == nil {
		return
	}
	w.onChange = fn
}

func (w *TextInput) changed() {
	if w.onChange != nil {
		w.onChange(w.value)
	}
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
		if w.showingCursor() && r.Width() > 0 {
			r.Set(0, 0, styled(" ", w.cursorStyle))
		}
		return
	}

	w.ensureCursorVisible(r.Width())
	visible := visibleFromCell(w.value, w.scroll, r.Width())
	drawText(r, 0, 0, visible, w.style)
	if !w.showingCursor() || r.Width() <= 0 {
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

// Handle edits the input for key and paste events. A read-only input consumes
// nothing, so its container can react to the same keys.
func (w *TextInput) Handle(ev tui.Event) bool {
	if w == nil || w.readOnly {
		return false
	}
	switch ev := ev.(type) {
	case input.TickEvent:
		if w.focused {
			w.cursorVisible = !w.cursorVisible
			return true
		}
		return false
	case input.PasteEvent:
		if ev.Text != "" {
			w.insert(ev.Text)
			w.showCursor()
			w.changed()
			return true
		}
		return false
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		switch ev.Key {
		case input.KeyRune:
			w.insert(string(ev.Rune))
			w.showCursor()
			w.changed()
			return true
		case input.KeyEnter:
			if w.onSubmit != nil {
				w.onSubmit(w.value)
				return true
			}
			return false
		case input.KeyLeft:
			if ev.Mods&input.Ctrl != 0 {
				w.cursor = previousWordBoundary(w.value, w.cursor)
				w.showCursor()
				return true
			}
			w.cursor = tuitext.PrevBoundary(w.value, w.cursor)
			w.showCursor()
			return true
		case input.KeyRight:
			if ev.Mods&input.Ctrl != 0 {
				w.cursor = nextWordBoundary(w.value, w.cursor)
				w.showCursor()
				return true
			}
			w.cursor = tuitext.NextBoundary(w.value, w.cursor)
			w.showCursor()
			return true
		case input.KeyUp:
			if ev.Mods&input.Ctrl != 0 {
				w.cursor = 0
				w.showCursor()
				return true
			}
		case input.KeyDown:
			if ev.Mods&input.Ctrl != 0 {
				w.cursor = len(w.value)
				w.showCursor()
				return true
			}
		case input.KeyHome:
			w.cursor = 0
			w.showCursor()
			return true
		case input.KeyEnd:
			w.cursor = len(w.value)
			w.showCursor()
			return true
		case input.KeyBackspace:
			prev := tuitext.PrevBoundary(w.value, w.cursor)
			if ev.Mods&input.Ctrl != 0 {
				prev = previousWordBoundary(w.value, w.cursor)
			}
			if prev != w.cursor {
				w.value = w.value[:prev] + w.value[w.cursor:]
				w.cursor = prev
				w.showCursor()
				w.changed()
			}
			return true
		case input.KeyDelete:
			next := tuitext.NextBoundary(w.value, w.cursor)
			if next != w.cursor {
				w.value = w.value[:w.cursor] + w.value[next:]
				w.showCursor()
				w.changed()
			}
			return true
		}
	}
	return false
}

func previousWordBoundary(value string, cursor int) int {
	i := cursor
	for i > 0 {
		prev := tuitext.PrevBoundary(value, i)
		if !isWhitespace(value[prev:i]) {
			break
		}
		i = prev
	}
	for i > 0 {
		prev := tuitext.PrevBoundary(value, i)
		if isWhitespace(value[prev:i]) {
			break
		}
		i = prev
	}
	return i
}

func nextWordBoundary(value string, cursor int) int {
	i := cursor
	for i < len(value) {
		next := tuitext.NextBoundary(value, i)
		if isWhitespace(value[i:next]) {
			break
		}
		i = next
	}
	for i < len(value) {
		next := tuitext.NextBoundary(value, i)
		if !isWhitespace(value[i:next]) {
			break
		}
		i = next
	}
	return i
}

func isWhitespace(cluster string) bool {
	for _, r := range cluster {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return cluster != ""
}

// Insert writes s at the cursor, advances past it, and fires the change
// callback. It is the programmatic equivalent of typing s, used by the picker to
// drop an emoji or URL into the composer.
func (w *TextInput) Insert(s string) {
	if w == nil || s == "" {
		return
	}
	w.insert(s)
	w.showCursor()
	w.changed()
}

// Replace replaces the byte range [start, end) with s. Both offsets are
// normalized to grapheme boundaries so callers cannot split a user-perceived
// character. The cursor is placed immediately after the replacement.
func (w *TextInput) Replace(start, end int, s string) {
	if w == nil || w.readOnly {
		return
	}
	start = replaceStartBoundary(w.value, start)
	end = replaceEndBoundary(w.value, end)
	if end < start {
		start, end = end, start
	}
	w.value = w.value[:start] + s + w.value[end:]
	w.cursor = start + len(s)
	w.scroll = 0
	w.showCursor()
	w.changed()
}

func replaceStartBoundary(value string, offset int) int {
	if offset <= 0 {
		return 0
	}
	if offset >= len(value) {
		return len(value)
	}
	for cluster := range tuitext.Clusters(value) {
		if offset <= cluster.Offset {
			return cluster.Offset
		}
		if offset < cluster.Offset+len(cluster.Text) {
			return cluster.Offset
		}
	}
	return len(value)
}

func replaceEndBoundary(value string, offset int) int {
	if offset <= 0 {
		return 0
	}
	if offset >= len(value) {
		return len(value)
	}
	for cluster := range tuitext.Clusters(value) {
		if offset <= cluster.Offset {
			return cluster.Offset
		}
		if offset < cluster.Offset+len(cluster.Text) {
			return cluster.Offset + len(cluster.Text)
		}
	}
	return len(value)
}

func (w *TextInput) insert(s string) {
	w.value = w.value[:w.cursor] + s + w.value[w.cursor:]
	w.cursor += len(s)
}

func (w *TextInput) showingCursor() bool {
	return w.focused && w.cursorVisible
}

func (w *TextInput) showCursor() {
	w.cursorVisible = true
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
