package markup_test

import (
	"fmt"

	"awesomeProject/internal/markup"
)

// Parse resolves Discord entities and splits content into styled spans.
func ExampleParse() {
	res := markup.Resolver{
		Member:  func(id uint64) (string, bool) { return "alice", true },
		Channel: func(id uint64) (string, bool) { return "general", true },
	}
	for _, span := range markup.Parse("hey <@42>, see <#7> **now**", res) {
		fmt.Printf("%d %q\n", span.Kind, span.Text)
	}
	// Output:
	// 0 "hey "
	// 6 "@alice"
	// 0 ", see "
	// 7 "#general"
	// 0 " "
	// 1 "now"
}
