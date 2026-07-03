package ui

import (
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
)

// drawText draws s at (x, y) within r, advancing by grapheme cluster width and
// stopping at the region's right edge. It returns the next free column.
func drawText(r screen.Region, x, y int, s string, style screen.Style) int {
	if y < 0 || y >= r.Height() || x >= r.Width() {
		return x
	}
	col := x
	for cluster := range text.Clusters(s) {
		if col >= r.Width() {
			break
		}
		if cluster.Width == 0 {
			continue
		}
		if cluster.Width == 2 && col+1 >= r.Width() {
			break
		}
		r.Set(col, y, screen.Cell{Content: cluster.Text, Style: style})
		col += cluster.Width
	}
	return col
}

// fill paints the whole region with spaces in style.
func fill(r screen.Region, style screen.Style) {
	r.Fill(screen.Rect{W: r.Width(), H: r.Height()}, screen.Cell{Content: " ", Style: style})
}
