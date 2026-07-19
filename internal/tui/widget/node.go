package widget

import (
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

// Node is a lightweight composable container widget.
type Node struct {
	node     layout.Node
	children []tui.Widget
}

// NewNode returns a container with dir and children.
func NewNode(dir layout.Direction, children ...tui.Widget) *Node {
	n := &Node{node: layout.Node{Dir: dir, Grow: 1}}
	n.SetChildren(children...)
	return n
}

// Row returns a horizontal container.
func Row(children ...tui.Widget) *Node {
	return NewNode(layout.Row, children...)
}

// Column returns a vertical container.
func Column(children ...tui.Widget) *Node {
	return NewNode(layout.Column, children...)
}

// SetChildren replaces the retained children.
func (n *Node) SetChildren(children ...tui.Widget) {
	if n == nil {
		return
	}
	n.children = append(n.children[:0], children...)
	n.node.Children = n.node.Children[:0]
	for _, child := range n.children {
		if child != nil {
			n.node.Children = append(n.node.Children, child.Layout())
		}
	}
}

// SetGap sets the gap between children and returns n.
func (n *Node) SetGap(gap int) *Node {
	if n == nil {
		return nil
	}
	n.node.Gap = gap
	return n
}

// Measure returns the available size. Child sizing is controlled by Layout.
func (n *Node) Measure(avail tui.Size) tui.Size {
	return avail
}

// Layout returns the node layout policy.
func (n *Node) Layout() *layout.Node {
	if n == nil {
		return nil
	}
	return &n.node
}

// Draw intentionally does nothing; children draw themselves.
func (n *Node) Draw(screen.Region) {}

// Handle offers events to children from front to back.
func (n *Node) Handle(ev tui.Event) bool {
	if n == nil {
		return false
	}
	if _, ok := ev.(input.MouseEvent); ok {
		return false
	}
	for i := len(n.children) - 1; i >= 0; i-- {
		if n.children[i] != nil && n.children[i].Handle(ev) {
			return true
		}
	}
	return false
}

// HandleBubble is intentionally inert: child dispatch is owned by the runtime
// focus path, not by a broadcast across every retained child.
func (n *Node) HandleBubble(tui.Event) bool { return false }

// Children returns the retained child widgets.
func (n *Node) Children() []tui.Widget {
	if n == nil {
		return nil
	}
	return n.children
}
