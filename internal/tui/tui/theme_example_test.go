package tui_test

import (
	"fmt"

	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

// WithTheme carries a palette on the App for the code that builds the widget tree.
func ExampleWithTheme() {
	theme := tui.Theme{
		Accent: screen.Style{Attrs: screen.Bold},
		Error:  screen.Style{Fg: screen.RGB(255, 0, 0)},
	}
	app := tui.New(tui.WithTheme(theme))

	fmt.Println(app.Theme().Error.Fg == screen.RGB(255, 0, 0))
	// Output:
	// true
}
