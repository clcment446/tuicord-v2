package screen

import "testing"

func TestSetWideCell(t *testing.T) {
	b := NewBuffer(4, 1)
	b.Set(1, 0, Cell{Content: "🎉"})
	if got := b.Cell(1, 0); got.Content != "🎉" || !got.Wide {
		t.Fatalf("wide cell = %+v", got)
	}
	if got := b.Cell(2, 0); !got.continuation {
		t.Fatalf("right half = %+v, want continuation", got)
	}
}

func TestSetClearsOverlappedWideCell(t *testing.T) {
	b := NewBuffer(4, 1)
	b.Set(1, 0, Cell{Content: "🎉"})
	b.Set(2, 0, Cell{Content: "x"})
	if got := b.Cell(1, 0); got.Content != " " || got.Wide {
		t.Fatalf("left half after overwrite = %+v", got)
	}
	if got := b.Cell(2, 0); got.Content != "x" || got.Wide || got.continuation {
		t.Fatalf("new cell = %+v", got)
	}
}

func TestSetWideAtRightEdgeBlanks(t *testing.T) {
	b := NewBuffer(2, 1)
	b.Set(1, 0, Cell{Content: "🎉"})
	if got := b.Cell(1, 0); got != Blank {
		t.Fatalf("edge wide cell = %+v, want blank", got)
	}
}

func TestFillAndClip(t *testing.T) {
	b := NewBuffer(4, 3)
	b.Fill(Rect{X: 1, Y: 1, W: 10, H: 10}, Cell{Content: "x"})
	if got := b.Cell(0, 0).Content; got != " " {
		t.Fatalf("outside fill = %q", got)
	}
	if got := b.Cell(3, 2).Content; got != "x" {
		t.Fatalf("inside fill = %q", got)
	}

	r := b.Clip(Rect{X: 1, Y: 0, W: 2, H: 1})
	r.Set(1, 0, Cell{Content: "y"})
	if got := b.Cell(2, 0).Content; got != "y" {
		t.Fatalf("region set = %q", got)
	}
	r.Set(2, 0, Cell{Content: "z"})
	if got := b.Cell(3, 0).Content; got != " " {
		t.Fatalf("out-of-region set = %q", got)
	}
}

func TestSetLayerStampsCells(t *testing.T) {
	b := NewBuffer(2, 1)
	b.Set(0, 0, Cell{Content: "a"})
	b.SetLayer(1)
	b.Set(1, 0, Cell{Content: "b"})
	if b.cellLayer[0] != 0 {
		t.Fatalf("cell 0 layer = %d, want 0", b.cellLayer[0])
	}
	if b.cellLayer[1] != 1 {
		t.Fatalf("cell 1 layer = %d, want 1", b.cellLayer[1])
	}
}

func TestClearResetsLayerStamps(t *testing.T) {
	b := NewBuffer(1, 1)
	b.SetLayer(1)
	b.Set(0, 0, Cell{Content: "a"})
	b.Clear()
	if b.layer != 0 {
		t.Fatalf("layer = %d, want 0 after Clear", b.layer)
	}
	if b.cellLayer[0] != 0 {
		t.Fatalf("cell layer = %d, want 0 after Clear", b.cellLayer[0])
	}
}
