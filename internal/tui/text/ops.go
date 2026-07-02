package text

import "strings"

// Ellipsis is the default truncation tail.
const Ellipsis = "…"

// Truncate shortens s to at most maxWidth cells, appending tail (measured
// within the budget) when anything was cut. Cuts happen only on grapheme
// boundaries, so combining marks, ZWJ emoji, and flags are never split.
func Truncate(s string, maxWidth int, tail string) string {
	if maxWidth <= 0 {
		return ""
	}
	if Width(s) <= maxWidth {
		return s
	}
	budget := max(maxWidth-Width(tail), 0)
	var b strings.Builder
	used := 0
	for c := range Clusters(s) {
		if used+c.Width > budget {
			break
		}
		b.WriteString(c.Text)
		used += c.Width
	}
	b.WriteString(tail)
	return b.String()
}

// PadRight pads s with spaces to exactly width cells, truncating with an
// ellipsis if it is too long. This is the primitive for fixed columns:
// always writing the full width prevents ghost cells from previous frames.
func PadRight(s string, width int) string {
	return pad(s, width, false)
}

// PadLeft right-aligns s in exactly width cells (spaces on the left),
// truncating with an ellipsis if it is too long.
func PadLeft(s string, width int) string {
	return pad(s, width, true)
}

func pad(s string, width int, left bool) string {
	if width <= 0 {
		return ""
	}
	w := Width(s)
	if w > width {
		s = Truncate(s, width, Ellipsis)
		w = Width(s)
	}
	fill := strings.Repeat(" ", width-w)
	if left {
		return fill + s
	}
	return s + fill
}

// ExpandTabs replaces each tab with spaces up to the next tab stop. Do this
// before any measuring or drawing — tab width is cursor-position dependent,
// so the layout code, not the terminal, must decide the spacing.
func ExpandTabs(s string, tabWidth int) string {
	if tabWidth <= 0 {
		tabWidth = 4
	}
	if !strings.ContainsRune(s, '\t') {
		return s
	}
	var b strings.Builder
	col := 0
	for c := range Clusters(s) {
		if c.Text == "\t" {
			n := tabWidth - col%tabWidth
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		if c.Text == "\n" {
			col = 0
			b.WriteString(c.Text)
			continue
		}
		b.WriteString(c.Text)
		col += c.Width
	}
	return b.String()
}

// Wrap breaks s into lines of at most width cells. Existing newlines are
// respected; wrapping prefers space boundaries and falls back to hard
// breaks on grapheme boundaries for unbroken runs (URLs, CJK). Zero-width
// runes travel with their base cluster, never alone at a line start.
func Wrap(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	var lines []string
	for para := range strings.SplitSeq(s, "\n") {
		lines = append(lines, wrapLine(para, width)...)
	}
	return lines
}

func wrapLine(s string, width int) []string {
	var lines []string
	var line strings.Builder
	lineWidth := 0
	spaceAt := -1 // byte length of line up to (excluding) last space
	widthAt := 0
	for c := range Clusters(s) {
		if lineWidth+c.Width > width && lineWidth > 0 {
			if c.Text == " " {
				// The break lands exactly on a space: emit the full line and
				// swallow the space instead of backtracking a word.
				lines = append(lines, line.String())
				line.Reset()
				lineWidth = 0
				spaceAt = -1
				continue
			}
			if spaceAt >= 0 {
				full := line.String()
				lines = append(lines, full[:spaceAt])
				rest := strings.TrimPrefix(full[spaceAt:], " ")
				line.Reset()
				line.WriteString(rest)
				lineWidth = lineWidth - widthAt - 1
			} else {
				lines = append(lines, line.String())
				line.Reset()
				lineWidth = 0
			}
			spaceAt = -1
		}
		if c.Text == " " {
			spaceAt = line.Len()
			widthAt = lineWidth
		}
		line.WriteString(c.Text)
		lineWidth += c.Width
	}
	return append(lines, line.String())
}
