package text_test

import (
	"fmt"

	"awesomeProject/internal/tui/text"
)

// Width measures display cells, which is the only count that matters for
// terminal layout. Bytes and runes disagree with it as soon as a user types
// CJK or emoji.
func ExampleWidth() {
	s := "mira🎉テスト"
	fmt.Println("bytes:", len(s))
	fmt.Println("cells:", text.Width(s))
	// Output:
	// bytes: 17
	// cells: 12
}

// A ZWJ family emoji is five runes but one grapheme drawn in two cells.
// Measuring it any other way overflows the column.
func ExampleWidth_zwjSequence() {
	family := "👨‍👩‍👧"
	fmt.Println("runes:", len([]rune(family)))
	fmt.Println("cells:", text.Width(family))
	// Output:
	// runes: 5
	// cells: 2
}

// Variation selectors flip a glyph between text and emoji presentation,
// changing its width. Discord users paste both forms constantly.
func ExampleClusterWidth() {
	fmt.Println("bare heart:    ", text.ClusterWidth("❤"))
	fmt.Println("emoji heart:   ", text.ClusterWidth("❤️"))
	fmt.Println("text hourglass:", text.ClusterWidth("⌛︎"))
	// Output:
	// bare heart:     1
	// emoji heart:    2
	// text hourglass: 1
}

// Clusters is the iteration primitive for anything that walks user text:
// cursor movement, truncation, hit testing. Never range over runes for
// those jobs.
func ExampleClusters() {
	for c := range text.Clusters("a❤️b") {
		fmt.Printf("%q offset=%d width=%d\n", c.Text, c.Offset, c.Width)
	}
	// Output:
	// "a" offset=0 width=1
	// "❤️" offset=1 width=2
	// "b" offset=7 width=1
}

// Truncate cuts on grapheme boundaries only, so an emoji is either kept
// whole or dropped — never sliced into a broken half.
func ExampleTruncate() {
	fmt.Println(text.Truncate("hello world", 8, text.Ellipsis))
	fmt.Println(text.Truncate("ab👨‍👩‍👧cd", 4, text.Ellipsis))
	// Output:
	// hello w…
	// ab…
}

// PadRight is the column primitive: the result is always exactly the
// requested number of cells, which also guarantees stale frame content is
// overwritten (no ghost cells).
func ExamplePadRight() {
	fmt.Printf("[%s]\n", text.PadRight("jules", 10))
	fmt.Printf("[%s]\n", text.PadRight("テスト", 10))
	fmt.Printf("[%s]\n", text.PadRight("a very long username", 10))
	// Output:
	// [jules     ]
	// [テスト    ]
	// [a very lo…]
}

// Wrap fills lines to a cell budget, preferring space boundaries and
// hard-breaking unbroken runs (URLs, CJK) on grapheme boundaries.
func ExampleWrap() {
	for _, line := range text.Wrap("hello brave new world", 11) {
		fmt.Printf("|%s|\n", line)
	}
	// Output:
	// |hello brave|
	// |new world|
}

// NextBoundary and PrevBoundary give correct cursor motion: one keypress
// moves over one user-perceived character, however many runes it holds.
func ExamplePrevBoundary() {
	s := "ab👨‍👩‍👧"
	// Backspace at the end deletes the whole family emoji, not one rune of it.
	fmt.Println(s[:text.PrevBoundary(s, len(s))])
	// Output:
	// ab
}
