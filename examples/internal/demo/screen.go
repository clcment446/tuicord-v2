package demo

import (
	"strings"

	"awesomeProject/internal/tui/screen"
)

// Dump returns a plain-text view of a screen buffer for noninteractive checks.
func Dump(buf *screen.Buffer) string {
	if buf == nil {
		return ""
	}
	var b strings.Builder
	for y := 0; y < buf.Height(); y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		for x := 0; x < buf.Width(); x++ {
			b.WriteString(buf.Cell(x, y).Content)
		}
	}
	return b.String()
}
