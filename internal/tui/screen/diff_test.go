package screen

import (
	"fmt"
	"testing"
)

func TestDiffFromEmpty(t *testing.T) {
	next := NewBuffer(3, 1)
	next.Set(1, 0, Cell{Content: "x"})
	got := string(Diff(nil, next))
	want := "\x1b[1;1H\x1b[0m x \x1b[0m"
	if got != want {
		t.Fatalf("Diff() = %q, want %q", got, want)
	}
}

func TestDiffSkipsUnchangedCells(t *testing.T) {
	prev := NewBuffer(4, 1)
	next := NewBuffer(4, 1)
	prev.Set(0, 0, Cell{Content: "a"})
	next.Set(0, 0, Cell{Content: "a"})
	next.Set(3, 0, Cell{Content: "b"})
	got := string(Diff(prev, next))
	want := "\x1b[1;4H\x1b[0mb\x1b[0m"
	if got != want {
		t.Fatalf("Diff() = %q, want %q", got, want)
	}
}

func TestDiffRepaintsEveryCellAfterResize(t *testing.T) {
	prev := NewBuffer(3, 1)
	prev.Set(0, 0, Cell{Content: "a"})
	prev.Set(1, 0, Cell{Content: "b"})
	next := NewBuffer(4, 1)
	next.Set(0, 0, Cell{Content: "a"})
	next.Set(1, 0, Cell{Content: "b"})

	got := string(Diff(prev, next))
	want := "\x1b[1;1H\x1b[0mab  \x1b[0m"
	if got != want {
		t.Fatalf("Diff() after resize = %q, want full repaint %q", got, want)
	}
}

func TestDiffStyle(t *testing.T) {
	next := NewBuffer(1, 1)
	next.Set(0, 0, Cell{
		Content: "x",
		Style:   Style{Fg: RGB(1, 2, 3), Attrs: Bold | Underline},
	})
	got := string(Diff(nil, next))
	want := "\x1b[1;1H\x1b[0;1;4;38;2;1;2;3mx\x1b[0m"
	if got != want {
		t.Fatalf("Diff() = %q, want %q", got, want)
	}
}

func TestDiffTTYColorsUsesANSI16Palette(t *testing.T) {
	next := NewBuffer(1, 1)
	next.Set(0, 0, Cell{
		Content: "x",
		Style:   Style{Fg: RGB(220, 40, 40), Bg: RGB(20, 20, 180), Attrs: Bold},
	})

	got := string(DiffWithColorMode(nil, next, ColorModeTTY16))
	want := "\x1b[1;1H\x1b[0;1;91;44mx\x1b[0m"
	if got != want {
		t.Fatalf("DiffWithColorMode() = %q, want %q", got, want)
	}
}

func TestDiffTTYColorsUsesProvidedPalette(t *testing.T) {
	next := NewBuffer(1, 1)
	next.Set(0, 0, Cell{Content: "x", Style: Style{Fg: RGB(1, 2, 3)}})
	palette := DefaultANSI16Palette()
	palette[1] = RGB(1, 2, 3)

	got := string(DiffWithPalette(nil, next, palette))
	want := "\x1b[1;1H\x1b[0;31mx\x1b[0m"
	if got != want {
		t.Fatalf("DiffWithPalette() = %q, want %q", got, want)
	}
}

func TestDiffTTYColorsUsesDefaultWhenColorIsUnset(t *testing.T) {
	next := NewBuffer(1, 1)
	next.Set(0, 0, Cell{Content: "x", Style: Style{Fg: RGB(255, 255, 255)}})

	got := string(DiffWithColorMode(nil, next, ColorModeTTY16))
	if got != "\x1b[1;1H\x1b[0;97mx\x1b[0m" {
		t.Fatalf("DiffWithColorMode() = %q, want ANSI 16-color foreground", got)
	}
}

func TestFrameSync(t *testing.T) {
	next := NewBuffer(1, 1)
	next.Set(0, 0, Cell{Content: "x"})
	got := string(Frame(nil, next, true))
	want := syncBegin + "\x1b[1;1H\x1b[0mx\x1b[0m" + syncEnd
	if got != want {
		t.Fatalf("Frame() = %q, want %q", got, want)
	}
}

func TestFrameEmitsGraphicsAfterCellDiff(t *testing.T) {
	next := NewBuffer(1, 1)
	next.Set(0, 0, Cell{Content: "x"})
	next.AddGraphic(Graphic{
		Key:        "image:1",
		PayloadKey: "payload:1",
		Rect:       Rect{W: 1, H: 1},
		Upload:     []byte("<upload>"),
		Data:       []byte("<image>"),
	})

	got := string(Frame(nil, next, false))
	want := "\x1b[1;1H\x1b[0mx\x1b[0m<upload><image>"
	if got != want {
		t.Fatalf("Frame() = %q, want %q", got, want)
	}
}

func TestFrameMovesGraphicsWithoutReuploading(t *testing.T) {
	prev := NewBuffer(2, 1)
	prev.AddGraphic(Graphic{
		Key:        "image:1",
		PayloadKey: "payload:1",
		Rect:       Rect{W: 1, H: 1},
		Clear:      []byte("<clear>"),
		Free:       []byte("<free>"),
		Upload:     []byte("<old-upload>"),
		Data:       []byte("<old>"),
	})
	next := NewBuffer(2, 1)
	next.Set(1, 0, Cell{Content: "x"})
	next.AddGraphic(Graphic{
		Key:        "image:1",
		PayloadKey: "payload:1",
		Rect:       Rect{X: 1, W: 1, H: 1},
		Clear:      []byte("<clear>"),
		Free:       []byte("<free>"),
		Upload:     []byte("<new-upload>"),
		Data:       []byte("<new>"),
	})

	got := string(Frame(prev, next, false))
	want := "\x1b[1;2H\x1b[0mx\x1b[0m<new>"
	if got != want {
		t.Fatalf("Frame() = %q, want %q", got, want)
	}
}

func TestFrameFreesRemovedGraphics(t *testing.T) {
	prev := NewBuffer(1, 1)
	prev.AddGraphic(Graphic{
		Key:        "image:1",
		PayloadKey: "payload:1",
		Rect:       Rect{W: 1, H: 1},
		Clear:      []byte("<clear>"),
		Free:       []byte("<free>"),
		Data:       []byte("<old>"),
	})
	next := NewBuffer(1, 1)

	got := string(Frame(prev, next, false))
	want := "<clear><free>"
	if got != want {
		t.Fatalf("Frame() = %q, want %q", got, want)
	}
}

func TestFrameUploadsSharedGraphicPayloadOnce(t *testing.T) {
	next := NewBuffer(2, 1)
	next.AddGraphic(Graphic{
		Key:        "image:1:1",
		PayloadKey: "payload:1",
		Rect:       Rect{W: 1, H: 1},
		Upload:     []byte("<upload>"),
		Data:       []byte("<one>"),
	})
	next.AddGraphic(Graphic{
		Key:        "image:1:2",
		PayloadKey: "payload:1",
		Rect:       Rect{X: 1, W: 1, H: 1},
		Upload:     []byte("<upload>"),
		Data:       []byte("<two>"),
	})

	got := string(Frame(nil, next, false))
	want := "\x1b[1;1H\x1b[0m  \x1b[0m<upload><one><two>"
	if got != want {
		t.Fatalf("Frame() = %q, want %q", got, want)
	}
}

func TestFrameDoesNotFreeSharedGraphicPayloadStillInUse(t *testing.T) {
	prev := NewBuffer(2, 1)
	prev.AddGraphic(Graphic{
		Key:        "image:1:1",
		PayloadKey: "payload:1",
		Rect:       Rect{W: 1, H: 1},
		Clear:      []byte("<clear-one>"),
		Free:       []byte("<free>"),
		Data:       []byte("<old-one>"),
	})
	prev.AddGraphic(Graphic{
		Key:        "image:1:2",
		PayloadKey: "payload:1",
		Rect:       Rect{X: 1, W: 1, H: 1},
		Clear:      []byte("<clear-two>"),
		Free:       []byte("<free>"),
		Data:       []byte("<old-two>"),
	})
	next := NewBuffer(2, 1)
	next.AddGraphic(Graphic{
		Key:        "image:1:2",
		PayloadKey: "payload:1",
		Rect:       Rect{X: 1, W: 1, H: 1},
		Clear:      []byte("<clear-two>"),
		Free:       []byte("<free>"),
		Data:       []byte("<old-two>"),
	})

	got := string(Frame(prev, next, false))
	want := "<clear-one>"
	if got != want {
		t.Fatalf("Frame() = %q, want %q", got, want)
	}
}

func TestGraphicOccludedByHigherLayerIsSuppressed(t *testing.T) {
	next := NewBuffer(3, 1)
	next.AddGraphic(Graphic{
		Key:        "image:1",
		PayloadKey: "payload:1",
		Rect:       Rect{W: 3, H: 1},
		Upload:     []byte("<upload>"),
		Data:       []byte("<image>"),
	})
	// An overlay on a higher layer paints a cell within the image's rect.
	next.SetLayer(1)
	next.Set(1, 0, Cell{Content: "x"})

	_, graphics := GraphicDiff(nil, next)
	if len(graphics) != 0 {
		t.Fatalf("occluded graphic should not be placed, got %q", graphics)
	}
}

func TestGraphicOccludedByOverlayEmitsClear(t *testing.T) {
	prev := NewBuffer(3, 1)
	prev.AddGraphic(Graphic{
		Key:        "image:1",
		PayloadKey: "payload:1",
		Rect:       Rect{W: 3, H: 1},
		Clear:      []byte("<clear>"),
		Free:       []byte("<free>"),
		Data:       []byte("<image>"),
	})
	next := NewBuffer(3, 1)
	next.AddGraphic(Graphic{
		Key:        "image:1",
		PayloadKey: "payload:1",
		Rect:       Rect{W: 3, H: 1},
		Clear:      []byte("<clear>"),
		Free:       []byte("<free>"),
		Data:       []byte("<image>"),
	})
	// Popup opens over the image the next frame.
	next.SetLayer(1)
	next.Set(1, 0, Cell{Content: "x"})

	clears, graphics := GraphicDiff(prev, next)
	if string(clears) != "<clear><free>" {
		t.Fatalf("clears = %q, want <clear><free>", clears)
	}
	if len(graphics) != 0 {
		t.Fatalf("graphics = %q, want none", graphics)
	}
}

func TestGraphicNotOccludedBySameLayerText(t *testing.T) {
	next := NewBuffer(3, 1)
	next.AddGraphic(Graphic{
		Key:        "image:1",
		PayloadKey: "payload:1",
		Rect:       Rect{W: 3, H: 1},
		Data:       []byte("<image>"),
	})
	// The play glyph is drawn on the same layer as the image; it must not occlude.
	next.Set(1, 0, Cell{Content: "▶"})

	_, graphics := GraphicDiff(nil, next)
	if string(graphics) != "<image>" {
		t.Fatalf("graphics = %q, want <image>", graphics)
	}
}

func TestGraphicNotOccludedByHigherLayerOutsideRect(t *testing.T) {
	next := NewBuffer(4, 1)
	next.AddGraphic(Graphic{
		Key:        "image:1",
		PayloadKey: "payload:1",
		Rect:       Rect{W: 2, H: 1},
		Data:       []byte("<image>"),
	})
	// A higher-layer cell outside the image rect leaves it visible.
	next.SetLayer(1)
	next.Set(3, 0, Cell{Content: "x"})

	_, graphics := GraphicDiff(nil, next)
	if string(graphics) != "<image>" {
		t.Fatalf("graphics = %q, want <image>", graphics)
	}
}

func reclipStub(visible []Rect) []byte {
	var b []byte
	for _, r := range visible {
		b = append(b, []byte(fmt.Sprintf("<sub %d+%d>", r.X, r.W))...)
	}
	return b
}

func TestGraphicPartiallyOccludedIsReclipped(t *testing.T) {
	next := NewBuffer(5, 1)
	next.AddGraphic(Graphic{
		Key:        "image:1",
		PayloadKey: "payload:1",
		Rect:       Rect{W: 5, H: 1},
		ClearAll:   []byte("<clearall>"),
		Data:       []byte("<whole>"),
		Reclip:     reclipStub,
	})
	// An overlay covers the middle column; the image shows on both sides.
	next.SetLayer(1)
	next.Set(2, 0, Cell{Content: "x"})

	_, graphics := GraphicDiff(nil, next)
	if want := "<sub 0+2><sub 3+2>"; string(graphics) != want {
		t.Fatalf("graphics = %q, want %q", string(graphics), want)
	}
}

func TestReclippedGraphicMoveEmitsClearAll(t *testing.T) {
	build := func(occludeX int) *Buffer {
		b := NewBuffer(5, 1)
		b.AddGraphic(Graphic{
			Key:        "image:1",
			PayloadKey: "payload:1",
			Rect:       Rect{W: 5, H: 1},
			ClearAll:   []byte("<clearall>"),
			Data:       []byte("<whole>"),
			Reclip:     reclipStub,
		})
		b.SetLayer(1)
		b.Set(occludeX, 0, Cell{Content: "x"})
		return b
	}
	prev := build(2)
	next := build(3)

	clears, graphics := GraphicDiff(prev, next)
	if want := "<clearall>"; string(clears) != want {
		t.Fatalf("clears = %q, want %q", string(clears), want)
	}
	if want := "<sub 0+3><sub 4+1>"; string(graphics) != want {
		t.Fatalf("graphics = %q, want %q", string(graphics), want)
	}
}

func TestReclippedGraphicUnchangedIsSkipped(t *testing.T) {
	build := func() *Buffer {
		b := NewBuffer(5, 1)
		b.AddGraphic(Graphic{
			Key:        "image:1",
			PayloadKey: "payload:1",
			Rect:       Rect{W: 5, H: 1},
			ClearAll:   []byte("<clearall>"),
			Data:       []byte("<whole>"),
			Reclip:     reclipStub,
		})
		b.SetLayer(1)
		b.Set(2, 0, Cell{Content: "x"})
		return b
	}
	clears, graphics := GraphicDiff(build(), build())
	if len(clears) != 0 || len(graphics) != 0 {
		t.Fatalf("unchanged occlusion should emit nothing, got clears=%q graphics=%q", clears, graphics)
	}
}
