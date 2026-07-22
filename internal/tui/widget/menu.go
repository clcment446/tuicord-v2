package widget

import (
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

// MenuItem is one entry in a Menu.
//
// A Separator item renders as a horizontal divider and is never selectable.
// A Disabled item is drawn dimmed and cannot be chosen (its OnSelect never
// fires). Key is an optional right-aligned shortcut hint (e.g. "Ctrl+E"); it is
// purely cosmetic. Danger tints the row to warn about a destructive action.
type MenuItem struct {
	// Label is the primary row text.
	Label string
	// Key is an optional right-aligned shortcut hint.
	Key string
	// Danger tints the row to mark a destructive action.
	Danger bool
	// Disabled dims the row and blocks selection.
	Disabled bool
	// Separator renders the entry as a divider line; Label and OnSelect are
	// ignored.
	Separator bool
	// OnSelect runs when the item is chosen (Enter or left click).
	OnSelect func()
}

func (it MenuItem) selectable() bool {
	return !it.Separator && !it.Disabled
}

// Menu is a lightweight popup list anchored at a screen cell. It is modal: it
// fills the available area, draws a bordered box at its (clamped) anchor, and
// captures every input event until dismissed. Selecting an item runs its
// OnSelect callback; pressing Esc or clicking outside the box runs OnDismiss.
// Both callbacks are the caller's cue to unmount the menu.
//
// The box clamps to the screen edges so it is always fully visible, even when
// anchored near the right or bottom. Menus are ordinary lists: arrow keys and
// Enter drive them from the keyboard, the mouse selects on hover and activates
// on click.
//
// Menu is intended to be mounted the way the quick switcher is — as the active
// overlay subtree — so it receives focus, hit testing, and drawing for the
// whole screen.
type Menu struct {
	items            []MenuItem
	selected         int
	anchorX, anchorY int
	screenW, screenH int
	last             screen.Rect
	onDismiss        func()

	style         screen.Style
	selectedStyle screen.Style
	dangerStyle   screen.Style
	disabledStyle screen.Style
	keyStyle      screen.Style
	borderStyle   screen.Style
	vimNavigation bool

	node layout.Node
}

// NewMenu returns a modal menu of items anchored at (0, 0). Use SetAnchor to
// position it at the pointer and OnDismiss to learn when it closes.
func NewMenu(items []MenuItem) *Menu {
	m := &Menu{
		selectedStyle: screen.Style{Attrs: screen.Reverse},
		keyStyle:      screen.Style{Attrs: screen.Dim},
		disabledStyle: screen.Style{Attrs: screen.Dim},
		dangerStyle:   screen.Style{Attrs: screen.Bold},
		node:          layout.Node{Grow: 1},
	}
	m.SetItems(items)
	return m
}

// SetItems replaces the menu entries and moves the selection to the first
// selectable row.
func (m *Menu) SetItems(items []MenuItem) {
	if m == nil {
		return
	}
	m.items = append(m.items[:0], items...)
	m.selected = m.firstSelectable()
}

// SetAnchor sets the desired top-left cell of the menu box. The box is clamped
// into the screen on draw, so anchoring past the right or bottom edge simply
// snaps it back inside.
func (m *Menu) SetAnchor(x, y int) {
	if m == nil {
		return
	}
	m.anchorX = maxInt(x, 0)
	m.anchorY = maxInt(y, 0)
}

// OnDismiss registers the callback fired when the menu is dismissed by Esc or a
// click outside its box. Passing nil clears it.
func (m *Menu) OnDismiss(fn func()) {
	if m == nil {
		return
	}
	m.onDismiss = fn
}

// Selected returns the selected row index, or -1 when nothing is selectable.
func (m *Menu) Selected() int {
	if m == nil {
		return -1
	}
	return m.selected
}

// SetStyle sets the style for normal (unselected) rows and the box background.
func (m *Menu) SetStyle(s screen.Style) {
	if m != nil {
		m.style = s
	}
}

// SetSelectedStyle sets the style for the highlighted row.
func (m *Menu) SetSelectedStyle(s screen.Style) {
	if m != nil {
		m.selectedStyle = s
	}
}

// SetDangerStyle sets the style for rows marked Danger.
func (m *Menu) SetDangerStyle(s screen.Style) {
	if m != nil {
		m.dangerStyle = s
	}
}

// SetDisabledStyle sets the style for Disabled rows.
func (m *Menu) SetDisabledStyle(s screen.Style) {
	if m != nil {
		m.disabledStyle = s
	}
}

// SetKeyStyle sets the style for the right-aligned key hints.
func (m *Menu) SetKeyStyle(s screen.Style) {
	if m != nil {
		m.keyStyle = s
	}
}

// SetBorderStyle sets the style for the box border.
func (m *Menu) SetBorderStyle(s screen.Style) {
	if m != nil {
		m.borderStyle = s
	}
}

// SetVimNavigation opts the menu into j/k movement. Arrow navigation remains
// available in every configuration.
func (m *Menu) SetVimNavigation(enabled bool) {
	if m != nil {
		m.vimNavigation = enabled
	}
}

// CanFocus reports that the menu takes keyboard focus while open.
func (m *Menu) CanFocus() bool { return m != nil }

// PreferredFocus makes the menu the initial focus owner when mounted.
func (m *Menu) PreferredFocus() bool { return true }

// Measure records the available screen size (used to clamp the box) and returns
// it unchanged; the menu spans the whole area as a modal layer.
func (m *Menu) Measure(avail tui.Size) tui.Size {
	if m == nil {
		return tui.Size{}
	}
	m.screenW = avail.W
	m.screenH = avail.H
	return avail
}

// Layout returns the full-area modal node.
func (m *Menu) Layout() *layout.Node {
	if m == nil {
		return nil
	}
	return &m.node
}

// box computes the clamped box rectangle for a screen of size w×h.
func (m *Menu) box(w, h int) screen.Rect {
	inner := 0
	for _, it := range m.items {
		if it.Separator {
			continue
		}
		row := text.Width(it.Label)
		if it.Key != "" {
			row += text.Width(it.Key) + 2 // gap before the key hint
		}
		inner = maxInt(inner, row)
	}
	// One space of padding on each side of the content, plus two border columns.
	boxW := minInt(inner+4, maxInt(w, 0))
	boxH := minInt(len(m.items)+2, maxInt(h, 0))
	x := m.anchorX
	y := m.anchorY
	if x+boxW > w {
		x = w - boxW
	}
	if y+boxH > h {
		y = h - boxH
	}
	return screen.Rect{X: maxInt(x, 0), Y: maxInt(y, 0), W: boxW, H: boxH}
}

// Draw renders the menu box within r. Cells outside the box are left untouched
// so any layer drawn behind the menu shows through.
func (m *Menu) Draw(r screen.Region) {
	if m == nil || r.Width() <= 0 || r.Height() <= 0 {
		return
	}
	rect := m.box(r.Width(), r.Height())
	m.last = rect
	if rect.W < 2 || rect.H < 2 {
		return
	}
	// Border box.
	for x := rect.X; x < rect.X+rect.W; x++ {
		r.Set(x, rect.Y, styled("─", m.borderStyle))
		r.Set(x, rect.Y+rect.H-1, styled("─", m.borderStyle))
	}
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		r.Set(rect.X, y, styled("│", m.borderStyle))
		r.Set(rect.X+rect.W-1, y, styled("│", m.borderStyle))
	}
	r.Set(rect.X, rect.Y, styled("┌", m.borderStyle))
	r.Set(rect.X+rect.W-1, rect.Y, styled("┐", m.borderStyle))
	r.Set(rect.X, rect.Y+rect.H-1, styled("└", m.borderStyle))
	r.Set(rect.X+rect.W-1, rect.Y+rect.H-1, styled("┘", m.borderStyle))

	innerW := rect.W - 2
	for i, it := range m.items {
		y := rect.Y + 1 + i
		if y >= rect.Y+rect.H-1 {
			break
		}
		if it.Separator {
			r.Set(rect.X, y, styled("├", m.borderStyle))
			for x := rect.X + 1; x < rect.X+rect.W-1; x++ {
				r.Set(x, y, styled("─", m.borderStyle))
			}
			r.Set(rect.X+rect.W-1, y, styled("┤", m.borderStyle))
			continue
		}
		m.drawRow(r, rect.X+1, y, innerW, i, it)
	}
}

func (m *Menu) drawRow(r screen.Region, x, y, innerW, index int, it MenuItem) {
	rowStyle := m.style
	keyStyle := m.keyStyle
	switch {
	case it.Disabled:
		rowStyle = m.disabledStyle
		keyStyle = m.disabledStyle
	case it.Danger:
		rowStyle = m.dangerStyle
	}
	if index == m.selected {
		rowStyle = m.selectedStyle
		keyStyle = m.selectedStyle
	}
	// Paint the whole inner row so the selection bar spans the box width.
	r.Fill(screen.Rect{X: x, Y: y, W: innerW, H: 1}, blank(rowStyle))
	keyW := 0
	if it.Key != "" {
		keyW = text.Width(it.Key)
	}
	labelW := maxInt(innerW-1-keyW-1, 0) // 1 leading space, 1 space before key
	drawText(r, x+1, y, text.Truncate(it.Label, labelW, text.Ellipsis), rowStyle)
	if keyW > 0 && keyW < innerW-1 {
		drawText(r, x+innerW-1-keyW, y, it.Key, keyStyle)
	}
}

// Handle processes keyboard and mouse events while the menu is open. It is
// modal: every event is consumed so nothing leaks to the layer behind it.
func (m *Menu) Handle(ev tui.Event) bool {
	if m == nil {
		return false
	}
	switch ev := ev.(type) {
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		return m.handleKey(ev)
	case input.MouseEvent:
		return m.handleMouse(ev)
	}
	return false
}

func (m *Menu) handleKey(ev input.KeyEvent) bool {
	switch ev.Key {
	case input.KeyUp:
		m.step(-1)
	case input.KeyDown:
		m.step(+1)
	case input.KeyHome:
		m.selected = m.firstSelectable()
	case input.KeyEnd:
		m.selected = m.lastSelectable()
	case input.KeyEnter:
		m.activate(m.selected)
	case input.KeyRune:
		switch ev.Rune {
		case 'j':
			if m.vimNavigation {
				m.step(+1)
			}
		case 'k':
			if m.vimNavigation {
				m.step(-1)
			}
		case ' ':
			m.activate(m.selected)
		}
	case input.KeyEsc:
		m.dismiss()
	}
	// Swallow all keys: a modal popup must not leak input to the layer behind it.
	return true
}

func (m *Menu) handleMouse(ev input.MouseEvent) bool {
	rect := m.last
	if rect.W == 0 {
		rect = m.box(m.screenW, m.screenH)
	}
	switch ev.Kind {
	case input.MousePress:
		idx, inside := m.itemAt(rect, ev.X, ev.Y)
		if !inside {
			m.dismiss()
			return true
		}
		if idx >= 0 && ev.Btn == input.ButtonLeft {
			m.selected = idx
			m.activate(idx)
		}
		return true
	case input.MouseMotion:
		if idx, inside := m.itemAt(rect, ev.X, ev.Y); inside && idx >= 0 {
			m.selected = idx
		}
		return true
	case input.MouseWheel:
		switch ev.Btn {
		case input.ButtonWheelUp:
			m.step(-1)
		case input.ButtonWheelDown:
			m.step(+1)
		}
		return true
	}
	return true
}

// itemAt maps a cell to a menu row. inside reports whether (x, y) lies within
// the box; idx is the selectable row index there, or -1 for the border, a
// separator, or a disabled row.
func (m *Menu) itemAt(rect screen.Rect, x, y int) (idx int, inside bool) {
	if x < rect.X || x >= rect.X+rect.W || y < rect.Y || y >= rect.Y+rect.H {
		return -1, false
	}
	row := y - rect.Y - 1
	// Only rows that Draw actually rendered are selectable. Draw stops before the
	// bottom border (rows with index >= rect.H-2 are not drawn), so when the menu
	// is clipped to the screen a click on the bottom border — or on an item past
	// the visible area — must not activate the entry that would sit there.
	if row < 0 || row >= rect.H-2 || row >= len(m.items) || !m.items[row].selectable() {
		return -1, true
	}
	return row, true
}

func (m *Menu) activate(index int) {
	if index < 0 || index >= len(m.items) {
		return
	}
	it := m.items[index]
	if !it.selectable() {
		return
	}
	if it.OnSelect != nil {
		it.OnSelect()
	}
	m.dismiss()
}

func (m *Menu) dismiss() {
	if m.onDismiss != nil {
		m.onDismiss()
	}
}

// step moves the selection by dir (±1), skipping separators and disabled rows,
// and stops at the ends without wrapping.
func (m *Menu) step(dir int) {
	if m.selected < 0 {
		m.selected = m.firstSelectable()
		return
	}
	for i := m.selected + dir; i >= 0 && i < len(m.items); i += dir {
		if m.items[i].selectable() {
			m.selected = i
			return
		}
	}
}

func (m *Menu) firstSelectable() int {
	for i := range m.items {
		if m.items[i].selectable() {
			return i
		}
	}
	return -1
}

func (m *Menu) lastSelectable() int {
	for i := len(m.items) - 1; i >= 0; i-- {
		if m.items[i].selectable() {
			return i
		}
	}
	return -1
}
