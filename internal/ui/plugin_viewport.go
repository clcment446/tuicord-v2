package ui

import (
	"awesomeProject/internal/plugin"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// pluginViewport is a compact, draggable panel owned by a Lua plugin. It is
// drawn as a popup so it never replaces the chat tree beneath it.
type pluginViewport struct {
	modal     *widget.Modal
	lines     []string
	actions   []plugin.ViewportAction
	onAction  func(string)
	textStyle screen.Style
	border    screen.Style
	last      screen.Rect
}

func newPluginViewport(title string, lines []string, actions []plugin.ViewportAction, onAction func(string), styles Styles) *pluginViewport {
	p := &pluginViewport{
		lines: append([]string(nil), lines...), actions: append([]plugin.ViewportAction(nil), actions...), onAction: onAction,
		textStyle: styles.Cell("messages.content"), border: styles.Cell("panels.border"),
	}
	p.modal = widget.NewModal(title, nil)
	p.modal.SetSize(54, min(18, max(7, len(lines)+4)))
	p.modal.SetStyle(p.border)
	return p
}

func (p *pluginViewport) Measure(avail tui.Size) tui.Size { return p.modal.Measure(avail) }
func (p *pluginViewport) Layout() *layout.Node            { return p.modal.Layout() }
func (p *pluginViewport) Children() []tui.Widget          { return nil }

func (p *pluginViewport) Draw(r screen.Region) {
	if p == nil || p.modal == nil {
		return
	}
	p.modal.Draw(r)
	p.last = p.modal.Bounds(tui.Size{W: r.Width(), H: r.Height()})
	if p.last.W < 3 || p.last.H < 1 {
		return
	}
	for y := p.last.Y + 1; y < p.last.Y+p.last.H-1; y++ {
		for x := p.last.X + 1; x < p.last.X+p.last.W-1; x++ {
			r.Set(x, y, screen.Cell{Content: " ", Style: p.textStyle})
		}
	}
	toggle := "[−]"
	if p.modal.Collapsed() {
		toggle = "[+]"
	}
	p.drawText(r, p.last.X+p.last.W-4, p.last.Y, toggle, p.border)
	if p.modal.Collapsed() {
		return
	}
	maxLines := max(0, p.last.H-4)
	for i, line := range p.lines {
		if i >= maxLines {
			break
		}
		p.drawText(r, p.last.X+1, p.last.Y+1+i, line, p.textStyle)
	}
	if len(p.actions) == 0 {
		return
	}
	x := p.last.X + 1
	y := p.last.Y + p.last.H - 2
	for _, action := range p.actions {
		label := "[" + action.Label + "]"
		if x+text.Width(label) > p.last.X+p.last.W-1 {
			break
		}
		p.drawText(r, x, y, label, p.border)
		x += text.Width(label) + 1
	}
}

func (p *pluginViewport) drawText(r screen.Region, x, y int, value string, style screen.Style) {
	if y < 0 || y >= r.Height() {
		return
	}
	value = text.Truncate(value, max(0, p.last.X+p.last.W-1-x), text.Ellipsis)
	for cluster := range text.Clusters(value) {
		if x >= r.Width() {
			break
		}
		r.Set(x, y, screen.Cell{Content: cluster.Text, Style: style})
		x += cluster.Width
	}
}

func (p *pluginViewport) Handle(ev tui.Event) bool {
	if p == nil {
		return false
	}
	mouse, ok := ev.(input.MouseEvent)
	if !ok || mouse.Kind != input.MousePress || mouse.Btn != input.ButtonLeft || !inside(p.last, mouse.X, mouse.Y) {
		return false
	}
	if mouse.Y == p.last.Y && mouse.X >= p.last.X+p.last.W-4 {
		p.modal.SetCollapsed(!p.modal.Collapsed())
		return true
	}
	if p.modal.Collapsed() || mouse.Y != p.last.Y+p.last.H-2 {
		return true
	}
	x := p.last.X + 1
	for _, action := range p.actions {
		width := text.Width(action.Label) + 2
		if mouse.X >= x && mouse.X < x+width {
			if p.onAction != nil {
				p.onAction(action.ID)
			}
			return true
		}
		x += width + 1
	}
	return true
}

func (p *pluginViewport) OverlayHit(x, y int) bool { return p != nil && inside(p.last, x, y) }

func (p *pluginViewport) DragStart(x, y int) (tui.DragOp, bool) {
	if p == nil || p.modal == nil {
		return nil, false
	}
	// The collapse control is a title-bar action, not a drag handle.
	if y == p.last.Y && x >= p.last.X+p.last.W-4 {
		return nil, false
	}
	return p.modal.DragStart(x, y)
}

func (p *pluginViewport) ResizeStart(x, y int) (tui.DragOp, bool) {
	if p == nil || p.modal == nil || p.modal.Collapsed() {
		return nil, false
	}
	return p.modal.ResizeStart(x, y)
}

func inside(rect screen.Rect, x, y int) bool {
	return x >= rect.X && y >= rect.Y && x < rect.X+rect.W && y < rect.Y+rect.H
}
