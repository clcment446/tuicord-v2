package screen

import "testing"

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
