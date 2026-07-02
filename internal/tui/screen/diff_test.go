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
