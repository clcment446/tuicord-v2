package term_test

import (
	"bytes"
	"fmt"

	"awesomeProject/internal/tui/term"
)

// CopyToClipboard writes an OSC 52 sequence to the terminal (here a buffer),
// falling back to wl-copy/xclip/xsel/pbcopy when no OSC 52 sink is available.
func ExampleCopyToClipboard() {
	var out bytes.Buffer
	_ = term.CopyToClipboard(&out, "message id 123")

	// The sequence starts with the OSC 52 introducer and ends with BEL.
	fmt.Printf("starts OSC 52: %v\n", bytes.HasPrefix(out.Bytes(), []byte("\x1b]52;c;")))
	fmt.Printf("ends with BEL: %v\n", out.Bytes()[out.Len()-1] == '\a')
	// Output:
	// starts OSC 52: true
	// ends with BEL: true
}
