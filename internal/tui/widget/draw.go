package widget

import (
	"strings"

	"awesomeProject/internal/tui/screen"
	tuitext "awesomeProject/internal/tui/text"
)

func blank(style screen.Style) screen.Cell {
	return screen.Cell{Content: " ", Style: style}
}

func styled(content string, style screen.Style) screen.Cell {
	return screen.Cell{Content: content, Style: style}
}

func clear(r screen.Region, style screen.Style) {
	r.Fill(screen.Rect{W: r.Width(), H: r.Height()}, blank(style))
}

func drawText(r screen.Region, x, y int, s string, style screen.Style) int {
	if y < 0 || y >= r.Height() || x >= r.Width() {
		return x
	}
	col := x
	for cluster := range tuitext.Clusters(s) {
		if col >= r.Width() {
			break
		}
		if cluster.Width == 0 {
			continue
		}
		if cluster.Width == 2 && col+1 >= r.Width() {
			break
		}
		r.Set(col, y, styled(cluster.Text, style))
		col += cluster.Width
	}
	return col
}

func drawPaddedText(r screen.Region, y int, s string, style screen.Style) {
	if y < 0 || y >= r.Height() || r.Width() <= 0 {
		return
	}
	clearLine(r, y, style)
	drawText(r, 0, y, tuitext.Truncate(s, r.Width(), tuitext.Ellipsis), style)
}

func clearLine(r screen.Region, y int, style screen.Style) {
	if y < 0 || y >= r.Height() {
		return
	}
	r.Fill(screen.Rect{Y: y, W: r.Width(), H: 1}, blank(style))
}

func visibleFromCell(s string, offset, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	col := 0
	for cluster := range tuitext.Clusters(s) {
		next := col + cluster.Width
		if next <= offset {
			col = next
			continue
		}
		if tuitext.Width(b.String())+cluster.Width > width {
			break
		}
		b.WriteString(cluster.Text)
		col = next
	}
	return b.String()
}

func cellOffsetOfByte(s string, off int) int {
	if off <= 0 {
		return 0
	}
	col := 0
	for cluster := range tuitext.Clusters(s) {
		if cluster.Offset >= off {
			break
		}
		col += cluster.Width
	}
	return col
}

func byteOffsetAtCell(s string, target int) int {
	if target <= 0 {
		return 0
	}
	col := 0
	last := 0
	for cluster := range tuitext.Clusters(s) {
		if col >= target {
			return cluster.Offset
		}
		col += cluster.Width
		last = cluster.Offset + len(cluster.Text)
	}
	return last
}

func clampInt(v, minValue, maxValue int) int {
	if v < minValue {
		return minValue
	}
	if maxValue > 0 && v > maxValue {
		return maxValue
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
