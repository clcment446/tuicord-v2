package widget_test

import (
	"fmt"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/widget"
)

// Tabs shows one Content at a time; arrows or strip clicks switch tabs.
func ExampleTabs() {
	tabs := widget.NewTabs([]widget.Tab{
		{Label: "Emoji", Content: widget.NewText("😀 grid")},
		{Label: "GIF", Content: widget.NewText("gif search")},
		{Label: "Sticker", Content: widget.NewText("sticker grid")},
	})

	fmt.Println("active:", tabs.Active())
	tabs.Handle(input.KeyEvent{Key: input.KeyRight})
	tabs.Handle(input.KeyEvent{Key: input.KeyRight})
	fmt.Println("active:", tabs.Active())
	// Output:
	// active: 0
	// active: 2
}
