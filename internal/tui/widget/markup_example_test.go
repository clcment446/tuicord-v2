package widget_test

import (
	"fmt"

	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/widget"
)

// Markup renders links, emphasis, files, and image attachments as styled plain
// text spans.
func ExampleMarkup() {
	m := widget.NewMarkup("open [docs]() ***bold** italic* __u__~~x~~ ![pic](./pic.png)")
	buf := screen.NewBuffer(29, 1)
	m.Draw(buf.Clip(buf.Bounds()))
	fmt.Println(row(buf, 0))
	// Output:
	// open docs bold italic ux pic
}
