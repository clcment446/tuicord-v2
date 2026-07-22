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
	modal       *widget.Modal
	lines       []string
	actions     []plugin.ViewportAction
	onAction    func(string)
	textStyle   screen.Style
	border      screen.Style
	last        screen.Rect
	userResized bool
}

type pluginViewportActionLayout struct {
	action plugin.ViewportAction
	label  string
	rect   screen.Rect
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
	if toggleRect, ok := p.toggleRect(); ok {
		toggle := "[−]"
		if p.modal.Collapsed() {
			toggle = "[+]"
		}
		p.drawText(r, toggleRect.X, toggleRect.Y, toggle, p.border)
	}
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
	for _, item := range p.actionLayout() {
		p.drawText(r, item.rect.X, item.rect.Y, item.label, p.border)
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
	if !ok || !inside(p.last, mouse.X, mouse.Y) {
		return false
	}
	// The viewport is opaque. Even mouse inputs it does not act on must stop
	// here rather than reaching covered chat/sidebar widgets.
	if mouse.Kind != input.MousePress || mouse.Btn != input.ButtonLeft {
		return true
	}
	if toggleRect, drawable := p.toggleRect(); drawable && inside(toggleRect, mouse.X, mouse.Y) {
		p.modal.SetCollapsed(!p.modal.Collapsed())
		return true
	}
	if p.modal.Collapsed() {
		return true
	}
	for _, item := range p.actionLayout() {
		if inside(item.rect, mouse.X, mouse.Y) {
			if p.onAction != nil {
				p.onAction(item.action.ID)
			}
			return true
		}
	}
	return true
}

func (p *pluginViewport) toggleRect() (screen.Rect, bool) {
	if p == nil || p.last.W < 5 || p.last.H < 1 {
		return screen.Rect{}, false
	}
	return screen.Rect{X: p.last.X + p.last.W - 4, Y: p.last.Y, W: 3, H: 1}, true
}

func (p *pluginViewport) actionLayout() []pluginViewportActionLayout {
	if p == nil || p.modal == nil || p.modal.Collapsed() || p.last.W < 3 || p.last.H < 3 {
		return nil
	}
	x := p.last.X + 1
	y := p.last.Y + p.last.H - 2
	right := p.last.X + p.last.W - 1
	items := make([]pluginViewportActionLayout, 0, len(p.actions))
	for _, action := range p.actions {
		label := "[" + action.Label + "]"
		width := text.Width(label)
		if x+width > right {
			break
		}
		items = append(items, pluginViewportActionLayout{
			action: action,
			label:  label,
			rect:   screen.Rect{X: x, Y: y, W: width, H: 1},
		})
		x += width + 1
	}
	return items
}

func (p *pluginViewport) OverlayHit(x, y int) bool { return p != nil && inside(p.last, x, y) }

func (p *pluginViewport) DragStart(x, y int) (tui.DragOp, bool) {
	if p == nil || p.modal == nil {
		return nil, false
	}
	// The collapse control is a title-bar action, not a drag handle.
	if toggleRect, drawable := p.toggleRect(); drawable && inside(toggleRect, x, y) {
		return nil, false
	}
	return p.modal.DragStart(x, y)
}

func (p *pluginViewport) ResizeStart(x, y int) (tui.DragOp, bool) {
	if p == nil || p.modal == nil || p.modal.Collapsed() {
		return nil, false
	}
	op, ok := p.modal.ResizeStart(x, y)
	if !ok {
		return nil, false
	}
	return &pluginViewportResize{viewport: p, op: op}, true
}

type pluginViewportResize struct {
	viewport *pluginViewport
	op       tui.DragOp
}

func (op *pluginViewportResize) DragMove(dx, dy int) {
	if op != nil && op.op != nil {
		op.op.DragMove(dx, dy)
	}
}

func (op *pluginViewportResize) DragEnd(commit bool) {
	if op == nil || op.op == nil {
		return
	}
	op.op.DragEnd(commit)
	if commit && op.viewport != nil {
		op.viewport.userResized = true
	}
}

func inside(rect screen.Rect, x, y int) bool {
	return x >= rect.X && y >= rect.Y && x < rect.X+rect.W && y < rect.Y+rect.H
}
