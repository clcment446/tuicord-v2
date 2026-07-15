package widget

import (
	"strings"
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
	scrollRow        int
	lastWidth        int
	multiline        bool
	maxRows          int
	focused          bool
	focusEnabled     bool
	readOnly         bool
	cursorVisible    bool
	preferredFocus   bool
	style            screen.Style
	placeholderStyle screen.Style
	cursorStyle      screen.Style
	onSubmit         func(string)
	onChange         func(string)
	onPaste          func(string) bool
	node             layout.Node
}

// SetMultiline enables wrapped, multi-row editing. The input grows to its
// content up to maxRows and then scrolls vertically. A non-positive maxRows
// restores the normal single-line behavior.
func (w *TextInput) SetMultiline(maxRows int) {
	if w == nil {
		return
	}
	w.multiline = maxRows > 1
	w.maxRows = maxRows
	if !w.multiline {
		w.scrollRow = 0
	}
}

// NewTextInput returns an empty text input with placeholder text.
func NewTextInput(placeholder string) *TextInput {
	return &TextInput{
		placeholder:      placeholder,
		focused:          true,
		focusEnabled:     true,
		cursorVisible:    true,
		preferredFocus:   true,
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
	return w != nil && w.focusEnabled && !w.readOnly
}

// SetInputFocusEnabled controls whether focus routing may enter this input without
// changing its contents or read-only state. Modal editors disable focus in
// normal mode and re-enable it only for explicit input mode.
// The name deliberately avoids tui.FocusConfigurable.SetFocusEnabled, which is
// reserved for globally configuring split selectors.
func (w *TextInput) SetInputFocusEnabled(enabled bool) {
	if w != nil {
		w.focusEnabled = enabled
	}
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
	return w != nil && w.preferredFocus
}

// SetPreferredFocus controls whether this input wins initial focus discovery.
func (w *TextInput) SetPreferredFocus(preferred bool) {
	if w != nil {
		w.preferredFocus = preferred
	}
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

// OnPaste can consume a bracketed-paste payload before it is inserted. It is
// useful for controls that accept a structured paste format in addition to
// normal text (for example workspace-file imports). Returning false preserves
// the ordinary text-paste behavior.
func (w *TextInput) OnPaste(fn func(string) bool) {
	if w != nil {
		w.onPaste = fn
	}
}

func (w *TextInput) changed() {
	if w.onChange != nil {
		w.onChange(w.value)
	}
}

// Measure returns the preferred input size.
func (w *TextInput) Measure(avail tui.Size) tui.Size {
	width := 1
	if w != nil {
		width = maxInt(tuitext.Width(w.value), tuitext.Width(w.placeholder))
	}
	if avail.W > 0 {
		width = minInt(width, avail.W)
	}
	height := 1
	if w != nil && w.multiline {
		height = strings.Count(w.value, "\n") + 1
		if height > w.maxRows {
			height = w.maxRows
		}
	}
	return tui.Size{W: maxInt(width, 1), H: height}
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
	if w.multiline {
		w.drawMultiline(r)
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

type inputRow struct{ start, end int }

func (w *TextInput) drawMultiline(r screen.Region) {
	w.lastWidth = r.Width()
	for y := 0; y < r.Height(); y++ {
		clearLine(r, y, w.style)
	}
	if r.Width() <= 0 {
		return
	}
	if w.value == "" {
		drawText(r, 0, 0, tuitext.Truncate(w.placeholder, r.Width(), tuitext.Ellipsis), w.placeholderStyle)
		if w.showingCursor() {
			r.Set(0, 0, styled(" ", w.cursorStyle))
		}
		return
	}
	rows := wrappedInputRows(w.value, r.Width())
	row, x := cursorInputPosition(w.value, rows, w.cursor)
	if row < w.scrollRow {
		w.scrollRow = row
	}
	if row >= w.scrollRow+r.Height() {
		w.scrollRow = row - r.Height() + 1
	}
	if w.scrollRow < 0 {
		w.scrollRow = 0
	}
	for y := 0; y < r.Height() && w.scrollRow+y < len(rows); y++ {
		line := rows[w.scrollRow+y]
		drawText(r, 0, y, w.value[line.start:line.end], w.style)
	}
	if !w.showingCursor() || row < w.scrollRow || row >= w.scrollRow+r.Height() || x >= r.Width() {
		return
	}
	content := " "
	if w.cursor < len(w.value) && w.value[w.cursor] != '\n' {
		for cluster := range tuitext.Clusters(w.value[w.cursor:]) {
			if cluster.Width > 0 {
				content = cluster.Text
			}
			break
		}
	}
	r.Set(x, row-w.scrollRow, styled(content, w.cursorStyle))
}

func wrappedInputRows(value string, width int) []inputRow {
	if width <= 0 {
		return []inputRow{{0, len(value)}}
	}
	rows := make([]inputRow, 0, strings.Count(value, "\n")+1)
	start, used := 0, 0
	for cluster := range tuitext.Clusters(value) {
		if cluster.Text == "\n" {
			rows = append(rows, inputRow{start, cluster.Offset})
			start, used = cluster.Offset+len(cluster.Text), 0
			continue
		}
		if used > 0 && used+cluster.Width > width {
			rows = append(rows, inputRow{start, cluster.Offset})
			start, used = cluster.Offset, 0
		}
		used += cluster.Width
	}
	rows = append(rows, inputRow{start, len(value)})
	return rows
}

func cursorInputPosition(value string, rows []inputRow, cursor int) (int, int) {
	for i, row := range rows {
		if cursor < row.start || cursor > row.end || (cursor == row.start && i > 0) {
			continue
		}
		return i, tuitext.Width(value[row.start:cursor])
	}
	last := len(rows) - 1
	return last, tuitext.Width(value[rows[last].start:rows[last].end])
}

func (w *TextInput) moveVertical(width, delta int) bool {
	if !w.multiline || width <= 0 {
		return false
	}
	rows := wrappedInputRows(w.value, width)
	row, x := cursorInputPosition(w.value, rows, w.cursor)
	target := row + delta
	if target < 0 || target >= len(rows) {
		return true
	}
	pos := rows[target].start
	used := 0
	for cluster := range tuitext.Clusters(w.value[rows[target].start:rows[target].end]) {
		if used+cluster.Width > x {
			break
		}
		used += cluster.Width
		pos = rows[target].start + cluster.Offset + len(cluster.Text)
	}
	w.cursor = pos
	w.showCursor()
	return true
}

// Handle edits the input for key and paste events. A read-only input consumes
// nothing, so its container can react to the same keys.
func (w *TextInput) Handle(ev tui.Event) bool {
	if w == nil || w.readOnly || !w.focusEnabled {
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
			if w.onPaste != nil && w.onPaste(ev.Text) {
				return true
			}
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
			if w.multiline && ev.Mods&input.Shift != 0 {
				w.insert("\n")
				w.showCursor()
				w.changed()
				return true
			}
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
			if w.moveVertical(w.lastWidth, -1) {
				return true
			}
			if ev.Mods&input.Ctrl != 0 {
				w.cursor = 0
				w.showCursor()
				return true
			}
		case input.KeyDown:
			if w.moveVertical(w.lastWidth, 1) {
				return true
			}
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
