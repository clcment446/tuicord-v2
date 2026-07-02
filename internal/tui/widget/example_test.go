package widget_test

import (
	"fmt"

	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/widget"
)

// List draws only the rows visible in its region.
func ExampleList_Draw() {
	list := widget.NewList([]string{"alpha", "beta", "gamma"})
	list.SetSelected(1)

	buf := screen.NewBuffer(5, 2)
	list.Draw(buf.Clip(buf.Bounds()))

	fmt.Println(row(buf, 0))
	fmt.Println(row(buf, 1))
	// Output:
	// alpha
	// beta
}

func row(buf *screen.Buffer, y int) string {
	out := ""
	for x := 0; x < buf.Width(); x++ {
		out += buf.Cell(x, y).Content
	}
	return out
}
