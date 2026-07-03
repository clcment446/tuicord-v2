package demo

import (
	"context"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

// Box is a small example-only container for composing widgets without adding
// more API to the library.
type Box struct {
	node     layout.Node
	children []tui.Widget
	cancel   context.CancelFunc
}

// Row returns a left-to-right container.
func Row(children ...tui.Widget) *Box {
	return newBox(layout.Row, children...)
}

// Column returns a top-to-bottom container.
func Column(children ...tui.Widget) *Box {
	return newBox(layout.Column, children...)
}

// WithCancel makes q, Escape, and Ctrl+C close the app.
func (b *Box) WithCancel(cancel context.CancelFunc) *Box {
	b.cancel = cancel
	return b
}

// Measure returns the available size. Examples lean on layout.Node policy.
func (b *Box) Measure(avail tui.Size) tui.Size {
	return avail
}

// Layout returns the box layout node.
func (b *Box) Layout() *layout.Node {
	return &b.node
}

// Draw intentionally does nothing; children draw themselves.
func (b *Box) Draw(screen.Region) {}

// Handle routes global quit keys.
func (b *Box) Handle(ev tui.Event) bool {
	if b == nil || b.cancel == nil {
		return false
	}
	key, ok := ev.(input.KeyEvent)
	if !ok || key.Release {
		return false
	}
	if key.Key == input.KeyEsc || (key.Key == input.KeyRune && key.Rune == 'q') ||
		(key.Key == input.KeyRune && key.Rune == 'c' && key.Mods&input.Ctrl != 0) {
		b.cancel()
		return true
	}
	return false
}

// Children returns child widgets for the retained tree.
func (b *Box) Children() []tui.Widget {
	if b == nil {
		return nil
	}
	return b.children
}

func newBox(dir layout.Direction, children ...tui.Widget) *Box {
	b := &Box{node: layout.Node{Dir: dir, Grow: 1}, children: children}
	for _, child := range children {
		if child == nil {
			continue
		}
		b.node.Children = append(b.node.Children, child.Layout())
	}
	return b
}
