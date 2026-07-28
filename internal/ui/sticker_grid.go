package ui

import (
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	tuitext "awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

const (
	stickerGridColumns = 3
	stickerGridPage    = 9
)

// StickerGrid renders one page of sticker previews as a keyboard- and
// mouse-navigable 3x3 grid.
type StickerGrid struct {
	items         []widget.Item
	selected      int
	style         screen.Style
	selectedStyle screen.Style
	borderStyle   screen.Style
	border        widget.BorderChars
	node          layout.Node
	viewW         int
	viewH         int
}

func NewStickerGrid(items []widget.Item, styles Styles, borderStyle string) *StickerGrid {
	g := &StickerGrid{
		style:         styles.Cell("picker"),
		selectedStyle: styles.Cell("picker.selected"),
		borderStyle:   styles.Cell("panels.border"),
		border:        widget.BorderCharsForStyle(borderStyle),
		node:          layout.Node{Grow: 1},
	}
	if g.selectedStyle == (screen.Style{}) {
		g.selectedStyle = screen.Style{Attrs: screen.Reverse}
	}
	g.SetItems(items)
	return g
}

func (g *StickerGrid) SetItems(items []widget.Item) {
	g.items = append(g.items[:0], items...)
	if len(g.items) == 0 {
		g.selected = 0
	} else if g.selected >= len(g.items) {
		g.selected = len(g.items) - 1
	}
}

func (g *StickerGrid) Selected() int {
	if g == nil || len(g.items) == 0 {
		return -1
	}
	return g.selected
}

func (g *StickerGrid) SetSelectedSilent(index int) {
	if g == nil || len(g.items) == 0 {
		return
	}
	g.selected = max(0, min(index, len(g.items)-1))
}

func (g *StickerGrid) CanFocus() bool       { return true }
func (g *StickerGrid) PreferredFocus() bool { return false }
func (g *StickerGrid) Layout() *layout.Node { return &g.node }
func (g *StickerGrid) Measure(avail tui.Size) tui.Size {
	return avail
}

func (g *StickerGrid) Draw(r screen.Region) {
	g.viewW, g.viewH = r.Width(), r.Height()
	r.Fill(screen.Rect{W: r.Width(), H: r.Height()}, screen.Cell{Content: " ", Style: g.style})
	if r.Width() < stickerGridColumns*3 || r.Height() < stickerGridColumns*3 {
		drawStickerGridText(r, 0, 0, "resize terminal for sticker preview", g.style)
		return
	}
	pageStart := (g.selected / stickerGridPage) * stickerGridPage
	for slot := 0; slot < stickerGridPage; slot++ {
		index := pageStart + slot
		if index >= len(g.items) {
			break
		}
		col, row := slot%stickerGridColumns, slot/stickerGridColumns
		x0, x1 := col*r.Width()/stickerGridColumns, (col+1)*r.Width()/stickerGridColumns
		y0, y1 := row*r.Height()/stickerGridColumns, (row+1)*r.Height()/stickerGridColumns
		g.drawCell(r.Clip(screen.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}), index)
	}
}

func (g *StickerGrid) drawCell(r screen.Region, index int) {
	if r.Width() < 2 || r.Height() < 2 {
		return
	}
	item := g.items[index]
	style := g.style
	if item.Style != (screen.Style{}) {
		style = item.Style
	}
	if index == g.selected {
		style = g.selectedStyle
	}
	r.Fill(screen.Rect{W: r.Width(), H: r.Height()}, screen.Cell{Content: " ", Style: style})
	b := g.border
	for x := 1; x < r.Width()-1; x++ {
		r.Set(x, 0, screen.Cell{Content: b.Horizontal, Style: g.borderStyle})
		r.Set(x, r.Height()-1, screen.Cell{Content: b.Horizontal, Style: g.borderStyle})
	}
	for y := 1; y < r.Height()-1; y++ {
		r.Set(0, y, screen.Cell{Content: b.Vertical, Style: g.borderStyle})
		r.Set(r.Width()-1, y, screen.Cell{Content: b.Vertical, Style: g.borderStyle})
	}
	r.Set(0, 0, screen.Cell{Content: b.TopLeft, Style: g.borderStyle})
	r.Set(r.Width()-1, 0, screen.Cell{Content: b.TopRight, Style: g.borderStyle})
	r.Set(0, r.Height()-1, screen.Cell{Content: b.BottomLeft, Style: g.borderStyle})
	r.Set(r.Width()-1, r.Height()-1, screen.Cell{Content: b.BottomRight, Style: g.borderStyle})

	inner := r.Clip(screen.Rect{X: 1, Y: 1, W: r.Width() - 2, H: r.Height() - 2})
	if graphic := item.Graphic; graphic != nil && graphic.Image != nil && inner.Height() > 1 {
		img := widget.NewKittyImageFrom(graphic.Image).
			SetID(graphic.ImageID).
			SetPlacementID(graphic.PlacementID).
			SetPixelSize(graphic.PixelWidth, graphic.PixelHeight).
			SetZ(graphic.Z).
			SetStyle(style)
		img.Draw(inner.Clip(screen.Rect{W: inner.Width(), H: inner.Height() - 1}))
	}
	label := tuitext.Truncate(item.Label, inner.Width(), tuitext.Ellipsis)
	drawStickerGridText(inner, 0, inner.Height()-1, label, style)
}

func (g *StickerGrid) Handle(ev tui.Event) bool {
	switch ev := ev.(type) {
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		switch ev.Key {
		case input.KeyLeft:
			g.SetSelectedSilent(g.selected - 1)
		case input.KeyRight:
			g.SetSelectedSilent(g.selected + 1)
		case input.KeyUp:
			g.SetSelectedSilent(g.selected - stickerGridColumns)
		case input.KeyDown:
			g.SetSelectedSilent(g.selected + stickerGridColumns)
		case input.KeyHome:
			g.SetSelectedSilent(0)
		case input.KeyEnd:
			g.SetSelectedSilent(len(g.items) - 1)
		case input.KeyPageUp:
			g.SetSelectedSilent(max(0, (g.selected/stickerGridPage-1)*stickerGridPage))
		case input.KeyPageDown:
			g.SetSelectedSilent((g.selected/stickerGridPage + 1) * stickerGridPage)
		default:
			return false
		}
		return true
	case input.MouseEvent:
		if ev.Kind != input.MousePress || ev.Btn != input.ButtonLeft || g.viewW <= 0 || g.viewH <= 0 {
			return false
		}
		col := min(stickerGridColumns-1, ev.X*stickerGridColumns/g.viewW)
		row := min(stickerGridColumns-1, ev.Y*stickerGridColumns/g.viewH)
		index := (g.selected/stickerGridPage)*stickerGridPage + row*stickerGridColumns + col
		if index < 0 || index >= len(g.items) {
			return false
		}
		g.selected = index
		return true
	}
	return false
}

func drawStickerGridText(r screen.Region, x, y int, value string, style screen.Style) {
	col := x
	for cluster := range tuitext.Clusters(value) {
		if col+cluster.Width > r.Width() {
			break
		}
		r.Set(col, y, screen.Cell{Content: cluster.Text, Style: style})
		col += cluster.Width
	}
}
