package widget_test

import (
	"fmt"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/widget"
)

// OnSubmit fires with the current value when the user presses Enter.
func ExampleTextInput_OnSubmit() {
	in := widget.NewTextInput("message")
	in.SetValue("hello")
	in.OnSubmit(func(value string) {
		fmt.Println("submitted:", value)
	})

	in.Handle(input.KeyEvent{Key: input.KeyEnter})
	// Output:
	// submitted: hello
}
