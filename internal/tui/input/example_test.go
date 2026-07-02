package input_test

import (
	"fmt"

	"awesomeProject/internal/tui/input"
)

// Parser decodes terminal byte sequences into typed events without doing any
// IO, which keeps input behavior easy to table-test.
func ExampleParser() {
	p := input.NewParser()
	for _, ev := range p.Feed([]byte("\x1b[<0;12;4M")) {
		mouse := ev.(input.MouseEvent)
		fmt.Println(mouse.X, mouse.Y, mouse.Btn, mouse.Kind)
	}
	// Output:
	// 11 3 1 0
}

// Bracketed paste is emitted as one PasteEvent, so pasted text cannot trigger
// keybindings one byte at a time.
func ExamplePasteEvent() {
	p := input.NewParser()
	for _, ev := range p.Feed([]byte("\x1b[200~hello\nworld\x1b[201~")) {
		fmt.Printf("%q\n", ev.(input.PasteEvent).Text)
	}
	// Output:
	// "hello\nworld"
}
