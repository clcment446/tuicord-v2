package widget

import (
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
	"image"
)

// Item is one row in an ItemList. Label is the primary text; Badge is optional
// trailing text (an unread count, a mention marker) drawn right-aligned. A zero
// Style falls back to the list's default row or selected style.
type Item struct {
	Label   string
	Badge   string
	Style   screen.Style
	Graphic *ItemGraphic
}

// ItemGraphic is an optional 2-cell Kitty graphic drawn over the start of a row.
type ItemGraphic struct {
	Image                         image.Image
	ImageID, PlacementID          uint32
	PixelWidth, PixelHeight, Cols int
	// Z is the Kitty z-index for the placement. Callers pass -1 to keep graphics
	// below text within their layer; cross-layer overlap is resolved by buffer
	// occlusion, not z (see screen.Buffer.SetLayer).
	Z int
}

// ItemList draws and navigates a virtualized list of styled rows. Unlike List,
// each row carries its own label, optional right-aligned badge, and per-row
// style override, so callers can render unread badges, mention counts, and
// muted/inactive rows without post-processing plain strings.
type ItemList struct {
	items         []Item
	selected      int
	offset        int
	style         screen.Style
	selectedStyle screen.Style
	badgeStyle    screen.Style
	onSelect      func(int)
	onHover       func(int)
	onVimFocus    func(bool) bool
	canDrag       func(int) bool
	onDrag        func(int, int) int
	onDrop        func(int, int)
	vimNavigation bool
	node          layout.Node
	viewH         int

	contextIndex int
	contextSet   bool
	dragItems    []Item
}

// NewItemList returns a list containing items.
func NewItemList(items []Item) *ItemList {
	w := &ItemList{
		selectedStyle: screen.Style{Attrs: screen.Reverse},
		badgeStyle:    screen.Style{Attrs: screen.Bold},
		node:          layout.Node{Grow: 1},
	}
	w.SetItems(items)
	return w
}

// Items returns a copy of the list rows.
func (w *ItemList) Items() []Item {
	if w == nil {
		return nil
	}
	out := make([]Item, len(w.items))
	copy(out, w.items)
	return out
}

// SetItems replaces the list rows and clamps selection.
func (w *ItemList) SetItems(items []Item) {
	if w == nil {
		return
	}
	w.items = append(w.items[:0], items...)
	if len(w.items) == 0 {
		w.selected = 0
		w.offset = 0
		return
	}
	w.selected = clampInt(w.selected, 0, len(w.items)-1)
}

// Selected returns the selected row index, or -1 when the list is empty.
func (w *ItemList) Selected() int {
	if w == nil || len(w.items) == 0 {
		return -1
	}
	return w.selected
}

// SetSelected selects row index after clamping it to the list bounds.
func (w *ItemList) SetSelected(index int) {
	w.setSelected(index, true)
}

// SetSelectedSilent selects row index without invoking the selection callback.
func (w *ItemList) SetSelectedSilent(index int) {
	w.setSelected(index, false)
}

func (w *ItemList) setSelected(index int, notify bool) {
	if w == nil || len(w.items) == 0 {
		return
	}
	prev := w.selected
	w.selected = clampInt(index, 0, len(w.items)-1)
	if notify && w.selected != prev && w.onSelect != nil {
		w.onSelect(w.selected)
	}
}

// SetStyle sets the style used for unselected rows.
func (w *ItemList) SetStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.style = style
}

// SetSelectedStyle sets the style used for the selected row.
func (w *ItemList) SetSelectedStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.selectedStyle = style
}

// SetBadgeStyle sets the style used for row badges on unselected rows.
func (w *ItemList) SetBadgeStyle(style screen.Style) {
	if w == nil {
		return
	}
	w.badgeStyle = style
}

// OnSelect registers a callback invoked with the new index whenever the
// selection changes. Passing nil clears the callback.
func (w *ItemList) OnSelect(fn func(int)) {
	if w == nil {
		return
	}
	w.onSelect = fn
}

// OnHover registers a callback for pointer motion over a visible row.
func (w *ItemList) OnHover(fn func(int)) {
	if w == nil {
		return
	}
	w.onHover = fn
}

// OnVimFocus installs a local h/l action. Returning true keeps focus on the
// list (for example after unfolding a selected category); false lets the TUI
// runtime traverse the normal focus ring like Shift+Tab/Tab.
func (w *ItemList) OnVimFocus(fn func(forward bool) bool) {
	if w != nil {
		w.onVimFocus = fn
	}
}

func (w *ItemList) SetDrag(can func(int) bool, drag func(int, int) int, drop func(int, int)) {
	if w == nil {
		return
	}
	w.canDrag = can
	w.onDrag = drag
	w.onDrop = drop
}

// SetVimNavigation opts this list into j/k and h/l navigation.
func (w *ItemList) SetVimNavigation(enabled bool) {
	if w != nil {
		w.vimNavigation = enabled
	}
}

func (w *ItemList) VimFocusEnabled() bool { return w != nil && w.vimNavigation }

// HandleVimFocus implements tui.VimFocusTraverser.
func (w *ItemList) HandleVimFocus(forward bool) bool {
	return w != nil && w.onVimFocus != nil && w.onVimFocus(forward)
}

// CanFocus reports that the list can receive keyboard focus.
func (w *ItemList) CanFocus() bool {
	return w != nil
}

// Measure returns the preferred size for visible list rows.
func (w *ItemList) Measure(avail tui.Size) tui.Size {
	if w == nil {
		return tui.Size{}
	}
	width := 0
	for _, item := range w.items {
		row := text.Width(item.Label)
		if item.Badge != "" {
			row += text.Width(item.Badge) + 1
		}
		width = maxInt(width, row)
	}
	if avail.W > 0 {
		width = minInt(width, avail.W)
	}
	height := len(w.items)
	if avail.H > 0 {
		height = minInt(height, avail.H)
	}
	return tui.Size{W: width, H: height}
}

// Layout returns the layout node for this list.
func (w *ItemList) Layout() *layout.Node {
	if w == nil {
		return nil
	}
	return &w.node
}

// Draw renders only the rows visible in r.
func (w *ItemList) Draw(r screen.Region) {
	if w == nil {
		return
	}
	clear(r, w.style)
	w.viewH = r.Height()
	w.ensureSelectedVisible(r.Height())
	for y := 0; y < r.Height(); y++ {
		index := w.offset + y
		if index >= len(w.items) {
			clearLine(r, y, w.style)
			continue
		}
		w.drawRow(r, y, index)
	}
}

func (w *ItemList) drawRow(r screen.Region, y, index int) {
	items := w.items
	if w.dragItems != nil {
		items = w.dragItems
	}
	item := items[index]
	selected := index == w.selected
	rowStyle := w.style
	if item.Style != (screen.Style{}) {
		rowStyle = item.Style
	}
	if selected {
		rowStyle = w.selectedStyle
	}
	clearLine(r, y, rowStyle)
	badgeW := 0
	if item.Badge != "" {
		badgeW = text.Width(item.Badge)
	}
	labelWidth := maxInt(r.Width()-badgeW-1, 0)
	if badgeW == 0 {
		labelWidth = r.Width()
	}
	drawText(r, 0, y, text.Truncate(item.Label, labelWidth, text.Ellipsis), rowStyle)
	if g := item.Graphic; g != nil && g.Image != nil {
		cols := g.Cols
		if cols <= 0 {
			cols = 2
		}
		img := NewKittyImageFrom(g.Image).SetID(g.ImageID).SetPlacementID(g.PlacementID).SetPixelSize(g.PixelWidth, g.PixelHeight).SetZ(g.Z).SetStyle(rowStyle)
		img.Draw(r.Clip(screen.Rect{X: 0, Y: y, W: cols, H: 1}))
	}
	if badgeW > 0 && badgeW < r.Width() {
		badgeStyle := w.badgeStyle
		if selected {
			badgeStyle = w.selectedStyle
		}
		drawText(r, r.Width()-badgeW, y, item.Badge, badgeStyle)
	}
}

// Handle changes selection for keyboard and wheel events. Navigation only
// moves the highlight; Enter or a mouse click activates the selected row.
func (w *ItemList) Handle(ev tui.Event) bool {
	if w == nil {
		return false
	}
	switch ev := ev.(type) {
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		switch ev.Key {
		case input.KeyRune:
			if ev.Mods != 0 {
				return false
			}
			if !w.vimNavigation {
				return false
			}
			switch ev.Rune {
			case 'j':
				w.SetSelectedSilent(w.selected + 1)
				return true
			case 'k':
				w.SetSelectedSilent(w.selected - 1)
				return true
			}
		case input.KeyEnter:
			if len(w.items) == 0 {
				return false
			}
			if w.onSelect != nil {
				w.onSelect(w.selected)
			}
			return true
		case input.KeyUp:
			w.SetSelectedSilent(w.selected - 1)
			return true
		case input.KeyDown:
			w.SetSelectedSilent(w.selected + 1)
			return true
		case input.KeyHome:
			w.SetSelectedSilent(0)
			return true
		case input.KeyEnd:
			w.SetSelectedSilent(len(w.items) - 1)
			return true
		case input.KeyPageUp:
			w.SetSelectedSilent(w.selected - 10)
			return true
		case input.KeyPageDown:
			w.SetSelectedSilent(w.selected + 10)
			return true
		}
	case input.MouseEvent:
		switch ev.Kind {
		case input.MouseMotion:
			index := w.offset + ev.Y
			if index >= 0 && index < len(w.items) && w.onHover != nil {
				w.onHover(index)
				return true
			}
		case input.MousePress:
			if ev.Btn == input.ButtonRight {
				index := w.offset + ev.Y
				if index < 0 || index >= len(w.items) {
					return false
				}
				// Record the row for the owner to open a context menu, but do
				// not consume the event: the owning shell intercepts the
				// right-click higher up (same contract as ChatView).
				w.contextIndex = index
				w.contextSet = true
				return false
			}
			if ev.Btn != input.ButtonLeft {
				return false
			}
			index := w.offset + ev.Y
			if index < 0 || index >= len(w.items) {
				return false
			}
			w.click(index)
			return true
		case input.MouseWheel:
			switch ev.Btn {
			case input.ButtonWheelUp:
				w.SetSelectedSilent(w.selected - 1)
				return true
			case input.ButtonWheelDown:
				w.SetSelectedSilent(w.selected + 1)
				return true
			}
		}
	}
	return false
}

func (w *ItemList) DragStart(x, y int) (tui.DragOp, bool) {
	if w == nil || w.onDrop == nil || x < 0 || y < 0 {
		return nil, false
	}
	index := w.offset + y
	if index < 0 || index >= len(w.items) || w.canDrag != nil && !w.canDrag(index) {
		return nil, false
	}
	drag := &listDrag{list: w, from: index, to: index, visual: index, startY: y, offset: w.offset, selected: w.selected}
	drag.items = append([]Item(nil), w.items...)
	w.dragItems = dragItems(drag.items, index, index)
	return drag, true
}

func (w *ItemList) click(index int) {
	same := index == w.selected
	w.SetSelected(index)
	if same && w.onSelect != nil {
		w.onSelect(index)
	}
}

type listDrag struct {
	list     *ItemList
	from     int
	to       int
	visual   int
	startY   int
	offset   int
	selected int
	moved    bool
	items    []Item
}

func (d *listDrag) DragMove(dx, dy int) {
	if d == nil || d.list == nil || len(d.list.items) == 0 {
		return
	}
	if dx != 0 || dy != 0 {
		d.moved = true
	}
	d.to = clampInt(d.offset+d.startY+dy, 0, len(d.list.items)-1)
	d.visual = d.to
	if d.list.onDrag != nil {
		d.visual = d.list.onDrag(d.from, d.to)
	}
	d.visual = clampInt(d.visual, 0, len(d.list.items)-1)
	d.list.dragItems = dragItems(d.items, d.from, d.visual)
	d.list.SetSelectedSilent(d.visual)
}

func (d *listDrag) DragEnd(commit bool) {
	if d == nil || d.list == nil {
		return
	}
	d.list.dragItems = nil
	if !commit {
		d.list.SetSelectedSilent(d.selected)
		return
	}
	if !d.moved {
		d.list.click(d.from)
		return
	}
	if d.list.onDrop != nil {
		d.list.onDrop(d.from, d.to)
	}
}

func dragItems(items []Item, from, to int) []Item {
	out := append([]Item(nil), items...)
	item := out[from]
	if from < to {
		copy(out[from:to], out[from+1:to+1])
	} else if from > to {
		copy(out[to+1:from+1], out[to:from])
	}
	item.Label = "> " + item.Label
	out[to] = item
	return out
}

// TakeContext returns and clears the row index most recently right-clicked. The
// owning widget calls it after a right-click to open a context menu; ok is false
// when no unhandled right-click is pending.
func (w *ItemList) TakeContext() (int, bool) {
	if w == nil || !w.contextSet {
		return 0, false
	}
	index := w.contextIndex
	w.contextSet = false
	return index, true
}

// ScrollExtent implements ScrollModel: the first visible row, the height drawn
// last frame, and the row count.
func (w *ItemList) ScrollExtent() (offset, viewport, content int) {
	if w == nil {
		return 0, 0, 0
	}
	return w.offset, w.viewH, len(w.items)
}

// ScrollTo implements ScrollModel by moving the first visible row. Selection
// follows into the visible window (silently) so the next Draw does not snap
// the list back to the selected row.
func (w *ItemList) ScrollTo(offset int) {
	if w == nil || len(w.items) == 0 {
		return
	}
	height := maxInt(w.viewH, 1)
	w.offset = clampInt(offset, 0, maxInt(len(w.items)-height, 0))
	w.setSelected(clampInt(w.selected, w.offset, w.offset+height-1), false)
}

func (w *ItemList) ensureSelectedVisible(height int) {
	if height <= 0 || len(w.items) == 0 {
		w.offset = 0
		return
	}
	if w.selected < w.offset {
		w.offset = w.selected
	}
	if w.selected >= w.offset+height {
		w.offset = w.selected - height + 1
	}
	w.offset = clampInt(w.offset, 0, maxInt(len(w.items)-height, 0))
}
