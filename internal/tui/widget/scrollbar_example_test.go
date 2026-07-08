package widget_test

import (
	"fmt"
	"strings"

	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/widget"
)

// Scrollbar tracks any ScrollModel; Viewport implements it out of the box.
func ExampleScrollbar() {
	vp := widget.NewViewport()
	vp.SetContent(strings.TrimSuffix(strings.Repeat("line\n", 40), "\n"))
	buf := screen.NewBuffer(20, 10)
	vp.Draw(buf.Clip(buf.Bounds()))

	bar := widget.NewScrollbar(vp)
	vp.ScrollTo(30)

	offset, viewport, content := vp.ScrollExtent()
	fmt.Printf("offset %d of %d, %d visible\n", offset, content, viewport)

	barBuf := screen.NewBuffer(1, 10)
	bar.Draw(barBuf.Clip(barBuf.Bounds()))
	var cells strings.Builder
	for y := range 10 {
		cells.WriteString(barBuf.Cell(0, y).Content)
	}
	fmt.Println(cells.String())
	// Output:
	// offset 30 of 40, 10 visible
	// ░░░░░░░░██
}
