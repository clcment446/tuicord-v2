package term_test

import (
	"fmt"

	"awesomeProject/internal/tui/term"
)

// Capabilities are conservative feature flags. Rendering code can use them to
// choose truecolor, synchronized output, or fallback escape sequences.
func ExampleCapabilities() {
	caps := term.Capabilities{TrueColor: true, Color256: true}
	fmt.Println(caps.TrueColor)
	fmt.Println(caps.Color256)
	// Output:
	// true
	// true
}

// Size is expressed in terminal cells, matching all layout and drawing math in
// the TUI library.
func ExampleSize() {
	sz := term.Size{Width: 80, Height: 24}
	fmt.Printf("%dx%d\n", sz.Width, sz.Height)
	// Output:
	// 80x24
}
