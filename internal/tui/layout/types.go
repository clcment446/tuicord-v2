// Package layout solves a small terminal-oriented flexbox model.
//
// The solver is deliberately pure: a tree of Nodes plus a container size
// produces Rects, with no drawing, terminal IO, or widget knowledge. Widgets
// can keep layout policy in their Node and let tests assert exact cell
// rectangles at common terminal sizes.
package layout

// Direction is the axis children are arranged on.
type Direction int

const (
	// Row places children from left to right.
	Row Direction = iota
	// Column places children from top to bottom.
	Column
)

// Insets are padding in terminal cells.
type Insets struct {
	Top, Right, Bottom, Left int
}

// Rect is a rectangle in terminal cells.
type Rect struct {
	X, Y int
	W, H int
}

// Node describes one layout box.
type Node struct {
	// Dir controls the child layout axis.
	Dir Direction
	// Grow receives a share of positive free space on the parent's main axis.
	Grow float64
	// Basis is the preferred main-axis size in cells.
	Basis int
	// Min is the minimum main-axis size in cells.
	Min int
	// Max is the maximum main-axis size in cells. Zero means unlimited.
	Max int
	// Gap is the number of cells between visible children.
	Gap int
	// Padding is inset from this node's rectangle to its children.
	Padding Insets
	// HideBelow hides this node when the root/container width is below it.
	HideBelow int
	// Children are laid out inside this node's padded content box.
	Children []*Node
}
