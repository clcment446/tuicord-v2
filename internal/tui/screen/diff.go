package screen

import (
	"bytes"
	"strconv"
)

const (
	syncBegin = "\x1b[?2026h"
	syncEnd   = "\x1b[?2026l"
)

// Diff returns ANSI bytes that transform prev into next. It moves the cursor
// only to changed runs and skips wide-cell continuations.
func Diff(prev, next *Buffer) []byte {
	var out bytes.Buffer
	emitDiff(&out, prev, next)
	return out.Bytes()
}

// Frame wraps Diff in synchronized output markers when sync is true.
func Frame(prev, next *Buffer, sync bool) []byte {
	diff := Diff(prev, next)
	if len(diff) == 0 {
		return nil
	}
	if !sync {
		return diff
	}
	out := make([]byte, 0, len(syncBegin)+len(diff)+len(syncEnd))
	out = append(out, syncBegin...)
	out = append(out, diff...)
	out = append(out, syncEnd...)
	return out
}

func emitDiff(out *bytes.Buffer, prev, next *Buffer) {
	if next == nil {
		return
	}
	var style Style
	styleSet := false
	for y := 0; y < next.h; y++ {
		x := 0
		for x < next.w {
			cell := next.Cell(x, y)
			if cell.continuation || equalCell(prevCell(prev, x, y), cell) {
				x++
				continue
			}
			move(out, x, y)
			for x < next.w {
				cell = next.Cell(x, y)
				if cell.continuation || equalCell(prevCell(prev, x, y), cell) {
					break
				}
				if !styleSet || cell.Style != style {
					sgr(out, cell.Style)
					style = cell.Style
					styleSet = true
				}
				out.WriteString(cell.Content)
				if cell.Wide {
					x += 2
				} else {
					x++
				}
			}
		}
	}
	if styleSet {
		out.WriteString("\x1b[0m")
	}
}

func prevCell(prev *Buffer, x, y int) Cell {
	if prev == nil {
		return Cell{}
	}
	return prev.Cell(x, y)
}

func equalCell(a, b Cell) bool {
	return a.Content == b.Content &&
		a.Wide == b.Wide &&
		a.Style == b.Style &&
		a.continuation == b.continuation
}

func move(out *bytes.Buffer, x, y int) {
	out.WriteString("\x1b[")
	out.WriteString(strconv.Itoa(y + 1))
	out.WriteByte(';')
	out.WriteString(strconv.Itoa(x + 1))
	out.WriteByte('H')
}

func sgr(out *bytes.Buffer, style Style) {
	out.WriteString("\x1b[0")
	if style.Attrs&Bold != 0 {
		out.WriteString(";1")
	}
	if style.Attrs&Dim != 0 {
		out.WriteString(";2")
	}
	if style.Attrs&Italic != 0 {
		out.WriteString(";3")
	}
	if style.Attrs&Underline != 0 {
		out.WriteString(";4")
	}
	if style.Attrs&Reverse != 0 {
		out.WriteString(";7")
	}
	if style.Attrs&Strike != 0 {
		out.WriteString(";9")
	}
	if style.Fg.Set() {
		out.WriteString(";38;2;")
		writeColor(out, style.Fg)
	}
	if style.Bg.Set() {
		out.WriteString(";48;2;")
		writeColor(out, style.Bg)
	}
	out.WriteByte('m')
}

func writeColor(out *bytes.Buffer, c Color) {
	out.WriteString(strconv.Itoa(int(c.R)))
	out.WriteByte(';')
	out.WriteString(strconv.Itoa(int(c.G)))
	out.WriteByte(';')
	out.WriteString(strconv.Itoa(int(c.B)))
}
