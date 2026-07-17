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
	clears, graphics := GraphicDiff(prev, next)
	if len(diff) == 0 && len(clears) == 0 && len(graphics) == 0 {
		return nil
	}
	frame := make([]byte, 0, len(clears)+len(diff)+len(graphics))
	frame = append(frame, clears...)
	frame = append(frame, diff...)
	frame = append(frame, graphics...)
	if !sync {
		return frame
	}
	out := make([]byte, 0, len(syncBegin)+len(frame)+len(syncEnd))
	out = append(out, syncBegin...)
	out = append(out, frame...)
	out = append(out, syncEnd...)
	return out
}

// GraphicDiff returns terminal protocol bytes needed to transform prev graphics
// into next graphics. Clears should be emitted before cell diffs; graphics
// should be emitted after cell diffs so terminal images sit on top of fallback
// cells.
func GraphicDiff(prev, next *Buffer) (clears, graphics []byte) {
	prevByKey := graphicMap(prev)
	prevPayloads := graphicPayloadSet(prev)
	nextGraphics := next.Graphics()
	nextByKey := make(map[string]Graphic, len(nextGraphics))
	nextPayloads := make(map[string]struct{}, len(nextGraphics))
	for _, g := range nextGraphics {
		nextByKey[g.Key] = g
		if g.PayloadKey != "" {
			nextPayloads[g.PayloadKey] = struct{}{}
		}
	}

	for key, old := range prevByKey {
		next, ok := nextByKey[key]
		if ok && equalGraphic(old, next) {
			continue
		}
		if !ok || old.PayloadKey != next.PayloadKey {
			clears = append(clears, old.Clear...)
			if _, stillUsed := nextPayloads[old.PayloadKey]; !stillUsed {
				clears = append(clears, old.Free...)
			}
		}
	}
	uploaded := map[string]struct{}{}
	for _, g := range nextGraphics {
		if old, ok := prevByKey[g.Key]; ok && equalGraphic(old, g) {
			continue
		}
		if _, alreadyPresent := prevPayloads[g.PayloadKey]; !alreadyPresent {
			if _, alreadyUploaded := uploaded[g.PayloadKey]; !alreadyUploaded {
				graphics = append(graphics, g.Upload...)
				uploaded[g.PayloadKey] = struct{}{}
			}
		} else if old, ok := prevByKey[g.Key]; ok && old.PayloadKey != g.PayloadKey {
			graphics = append(graphics, g.Upload...)
		}
		graphics = append(graphics, g.Data...)
	}
	return clears, graphics
}

func emitDiff(out *bytes.Buffer, prev, next *Buffer) {
	if next == nil {
		return
	}
	// A terminal is allowed to reflow its existing cells when its dimensions
	// change. The old buffer therefore no longer describes what is physically
	// on screen after a resize, even where its coordinates overlap next. Treat
	// every cell as changed so wrapping and borders are repainted in place.
	if prev != nil && (prev.w != next.w || prev.h != next.h) {
		prev = nil
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

func graphicMap(buf *Buffer) map[string]Graphic {
	graphics := buf.Graphics()
	out := make(map[string]Graphic, len(graphics))
	for _, g := range graphics {
		out[g.Key] = g
	}
	return out
}

func graphicPayloadSet(buf *Buffer) map[string]struct{} {
	graphics := buf.Graphics()
	out := make(map[string]struct{}, len(graphics))
	for _, g := range graphics {
		if g.PayloadKey != "" {
			out[g.PayloadKey] = struct{}{}
		}
	}
	return out
}

func equalGraphic(a, b Graphic) bool {
	return a.Key == b.Key &&
		a.PayloadKey == b.PayloadKey &&
		a.Rect == b.Rect &&
		bytes.Equal(a.Clear, b.Clear) &&
		bytes.Equal(a.Free, b.Free) &&
		bytes.Equal(a.Data, b.Data)
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
