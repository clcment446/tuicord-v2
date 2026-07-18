package media

import (
	"strings"
	"testing"
)

func TestKittyClearRegionEmitsPerCellDeletes(t *testing.T) {
	got := string(KittyClearRegion(Rect{X: 5, Y: 3, Cols: 2, Rows: 2}))
	if n := strings.Count(got, "a=d,d=p,x="); n != 4 {
		t.Fatalf("delete count = %d, want 4 (2x2 region)", n)
	}
	// Coordinates are 1-based; the top-left cell (0-based 5,3) becomes x=6,y=4.
	if !strings.Contains(got, "x=6,y=4") {
		t.Fatalf("missing top-left delete x=6,y=4 in %q", got)
	}
	if !strings.Contains(got, "x=7,y=5") {
		t.Fatalf("missing bottom-right delete x=7,y=5 in %q", got)
	}
}

func TestKittyClearRegionEmptyIsNil(t *testing.T) {
	if b := KittyClearRegion(Rect{X: 1, Y: 1}); b != nil {
		t.Fatalf("empty region produced %q, want nil", b)
	}
}
