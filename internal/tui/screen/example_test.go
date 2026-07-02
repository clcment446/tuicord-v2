package screen_test

import (
	"fmt"

	"awesomeProject/internal/tui/screen"
)

// Buffer stores grapheme clusters in terminal cells. Wide clusters occupy the
// left cell and reserve the next cell as an internal continuation.
func ExampleBuffer_Set() {
	b := screen.NewBuffer(4, 1)
	b.Set(1, 0, screen.Cell{Content: "🎉"})
	fmt.Println(b.Cell(1, 0).Content, b.Cell(1, 0).Wide)
	fmt.Println(b.Cell(2, 0).Content)
	// Output:
	// 🎉 true
	//
}

// Diff emits cursor moves and changed cells only.
func ExampleDiff() {
	next := screen.NewBuffer(3, 1)
	next.Set(1, 0, screen.Cell{Content: "x"})
	fmt.Printf("%q\n", string(screen.Diff(nil, next)))
	// Output:
	// "\x1b[1;1H\x1b[0m x \x1b[0m"
}
