package widget_test

import (
	"fmt"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/widget"
)

// A Menu is a modal popup anchored at the pointer. Selecting an item runs its
// OnSelect callback; Esc or a click outside runs OnDismiss. Both are the cue to
// unmount it.
func ExampleMenu() {
	open := true
	menu := widget.NewMenu([]widget.MenuItem{
		{Label: "Reply", Key: "R", OnSelect: func() { fmt.Println("reply") }},
		{Label: "Edit", Key: "E", Disabled: true},
		{Separator: true},
		{Label: "Delete", Key: "Del", Danger: true, OnSelect: func() { fmt.Println("delete") }},
	})
	menu.SetAnchor(40, 3)
	menu.OnDismiss(func() { open = false })

	// Move down once: skips the disabled "Edit" and the separator, landing on
	// "Delete", then activate it with Enter.
	menu.Handle(input.KeyEvent{Key: input.KeyDown})
	menu.Handle(input.KeyEvent{Key: input.KeyEnter})

	fmt.Println("open:", open)
	// Output:
	// delete
	// open: false
}
