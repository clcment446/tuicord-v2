package layout_test

import (
	"fmt"

	"awesomeProject/internal/tui/layout"
)

// Solve returns exact cell rectangles for a retained layout tree.
func ExampleSolve() {
	sidebar := &layout.Node{Basis: 20}
	chat := &layout.Node{Grow: 1}
	root := &layout.Node{
		Dir:      layout.Row,
		Gap:      1,
		Children: []*layout.Node{sidebar, chat},
	}
	rects := layout.Solve(root, 80, 24)
	fmt.Println(rects[sidebar])
	fmt.Println(rects[chat])
	// Output:
	// {0 0 20 24}
	// {21 0 59 24}
}
