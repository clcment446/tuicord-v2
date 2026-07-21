package screen

import (
	"bytes"
	"strconv"
)

const (
	syncBegin = "\x1b[?2026h"
	syncEnd   = "\x1b[?2026l"
)

// ColorMode controls how explicit RGB colors are encoded in ANSI output.
type ColorMode uint8

// Palette is the terminal's ANSI color table for indexes 0 through 15.
type Palette [16]Color

const (
	// ColorModeTrueColor preserves explicit RGB colors.
	ColorModeTrueColor ColorMode = iota
	// ColorModeTTY16 maps colors to the terminal's standard 16-color palette.
	ColorModeTTY16
)

// Diff returns ANSI bytes that transform prev into next. It moves the cursor
// only to changed runs and skips wide-cell continuations.
func Diff(prev, next *Buffer) []byte {
	return DiffWithColorMode(prev, next, ColorModeTrueColor)
}

// DiffWithColorMode returns ANSI bytes that transform prev into next using the
// requested color encoding.
func DiffWithColorMode(prev, next *Buffer, mode ColorMode) []byte {
	var out bytes.Buffer
	emitDiff(&out, prev, next, mode, DefaultANSI16Palette())
	return out.Bytes()
}

// DiffWithPalette emits ANSI16 output using the supplied terminal palette.
func DiffWithPalette(prev, next *Buffer, palette Palette) []byte {
	var out bytes.Buffer
	emitDiff(&out, prev, next, ColorModeTTY16, palette)
	return out.Bytes()
}

// Frame wraps Diff in synchronized output markers when sync is true.
func Frame(prev, next *Buffer, sync bool) []byte {
	return FrameWithColorMode(prev, next, sync, ColorModeTrueColor)
}

// FrameWithColorMode wraps a color-mode-aware Diff in synchronized output
// markers when sync is true.
func FrameWithColorMode(prev, next *Buffer, sync bool, mode ColorMode) []byte {
	diff := DiffWithColorMode(prev, next, mode)
	return frameWithDiff(prev, next, sync, diff)
}

// FrameWithPalette emits a frame using the supplied terminal ANSI16 palette.
func FrameWithPalette(prev, next *Buffer, sync bool, palette Palette) []byte {
	diff := DiffWithPalette(prev, next, palette)
	return frameWithDiff(prev, next, sync, diff)
}

func frameWithDiff(prev, next *Buffer, sync bool, diff []byte) []byte {
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
	// Occlusion: resolve both frames' graphics against their overlay coverage, so
	// a graphic covered by a higher layer is dropped (its Clear emitted) or
	// re-clipped around the overlay, and reappears whole once the overlay is gone.
	prevGraphics := prev.resolveGraphics()
	nextGraphics := next.resolveGraphics()
	prevByKey := graphicMap(prevGraphics)
	prevPayloads := graphicPayloadSet(prevGraphics)
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
		// A re-clipped (split) graphic occupies a variable set of placements, so
		// any change must delete them all first (old.Clear is ClearAll) before the
		// new bytes are placed — the same-payload fast path would otherwise leave
		// stale sub-placements behind.
		if !ok || old.PayloadKey != next.PayloadKey || old.split || next.split {
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

func emitDiff(out *bytes.Buffer, prev, next *Buffer, mode ColorMode, palette Palette) {
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
				encodedStyle := encodeStyle(cell.Style, mode, palette)
				if !styleSet || encodedStyle != style {
					sgr(out, encodedStyle, mode, palette)
					style = encodedStyle
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

func graphicMap(graphics []Graphic) map[string]Graphic {
	out := make(map[string]Graphic, len(graphics))
	for _, g := range graphics {
		out[g.Key] = g
	}
	return out
}

func graphicPayloadSet(graphics []Graphic) map[string]struct{} {
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

func encodeStyle(style Style, mode ColorMode, palette Palette) Style {
	if mode != ColorModeTTY16 {
		return style
	}
	if style.Fg.Set() {
		style.Fg = ansi16Color(style.Fg, palette)
	}
	if style.Bg.Set() {
		style.Bg = ansi16Color(style.Bg, palette)
	}
	return style
}

// DefaultANSI16Palette returns the conventional xterm ANSI16 palette.
func DefaultANSI16Palette() Palette {
	return Palette{
		RGB(0, 0, 0), RGB(128, 0, 0), RGB(0, 128, 0), RGB(128, 128, 0),
		RGB(0, 0, 128), RGB(128, 0, 128), RGB(0, 128, 128), RGB(192, 192, 192),
		RGB(128, 128, 128), RGB(255, 0, 0), RGB(0, 255, 0), RGB(255, 255, 0),
		RGB(0, 0, 255), RGB(255, 0, 255), RGB(0, 255, 255), RGB(255, 255, 255),
	}
}

func ansi16Color(c Color, palette Palette) Color {
	best := palette[0]
	bestDistance := colorDistance(c, best)
	for _, candidate := range palette[1:] {
		if distance := colorDistance(c, candidate); distance < bestDistance {
			best = candidate
			bestDistance = distance
		}
	}
	return best
}

func colorDistance(a, b Color) int {
	r := int(a.R) - int(b.R)
	g := int(a.G) - int(b.G)
	blue := int(a.B) - int(b.B)
	return r*r + g*g + blue*blue
}

func sgr(out *bytes.Buffer, style Style, mode ColorMode, palette Palette) {
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
		if mode == ColorModeTTY16 {
			writeANSIColor(out, style.Fg, false, palette)
		} else {
			out.WriteString(";38;2;")
			writeColor(out, style.Fg)
		}
	}
	if style.Bg.Set() {
		if mode == ColorModeTTY16 {
			writeANSIColor(out, style.Bg, true, palette)
		} else {
			out.WriteString(";48;2;")
			writeColor(out, style.Bg)
		}
	}
	out.WriteByte('m')
}

func writeANSIColor(out *bytes.Buffer, c Color, background bool, palette Palette) {
	index := ansi16Index(c, palette)
	if index < 8 {
		if background {
			out.WriteString(";")
			out.WriteString(strconv.Itoa(40 + int(index)))
		} else {
			out.WriteString(";")
			out.WriteString(strconv.Itoa(30 + int(index)))
		}
		return
	}
	if background {
		out.WriteString(";")
		out.WriteString(strconv.Itoa(100 + int(index-8)))
	} else {
		out.WriteString(";")
		out.WriteString(strconv.Itoa(90 + int(index-8)))
	}
}

func ansi16Index(c Color, palette Palette) int {
	for i, candidate := range palette {
		if c == candidate {
			return i
		}
	}
	return 0
}

func writeColor(out *bytes.Buffer, c Color) {
	out.WriteString(strconv.Itoa(int(c.R)))
	out.WriteByte(';')
	out.WriteString(strconv.Itoa(int(c.G)))
	out.WriteByte(';')
	out.WriteString(strconv.Itoa(int(c.B)))
}
