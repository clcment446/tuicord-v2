package media

import (
	"bytes"
	"strings"
	"testing"
)

func TestKittyOutputFramerKeepsContinuedImageAtomic(t *testing.T) {
	input := []byte("\x1b[1;1f\x1b_Ga=T,m=1;AAAA\x1b\\\x1b_Gm=1;BBBB\x1b\\\x1b_Gm=0;CCCC\x1b\\\x1b[2;1f")
	var framer kittyOutputFramer
	var got [][]byte
	for i := 0; i < len(input); i += 5 {
		end := min(i+5, len(input))
		got = append(got, framer.Push(input[i:end])...)
	}
	if tail := framer.Flush(); len(tail) > 0 {
		got = append(got, tail)
	}
	joined := bytes.Join(got, nil)
	if !bytes.Equal(joined, input) {
		t.Fatalf("framed output changed bytes\n got %q\nwant %q", joined, input)
	}
	imageWrites := 0
	for _, chunk := range got {
		if bytes.Contains(chunk, []byte("AAAA")) || bytes.Contains(chunk, []byte("BBBB")) || bytes.Contains(chunk, []byte("CCCC")) {
			imageWrites++
			for _, payload := range [][]byte{[]byte("AAAA"), []byte("BBBB"), []byte("CCCC")} {
				if !bytes.Contains(chunk, payload) {
					t.Fatalf("continued image split across writes: %q", chunk)
				}
			}
		}
	}
	if imageWrites != 1 {
		t.Fatalf("image writes = %d, want exactly one atomic write", imageWrites)
	}
}

func TestKittyOutputFramerEmitsSharedMemoryFrameDespiteM1(t *testing.T) {
	packet := []byte("\x1b_Ga=T,t=s,f=24,m=1;bXB2LWtpdHR5LXRlc3Q=\x1b\\")
	var framer kittyOutputFramer
	got := framer.Push(packet)
	if len(got) != 1 || !bytes.Equal(got[0], packet) {
		t.Fatalf("shared-memory frame was buffered: %#v", got)
	}
	if tail := framer.Flush(); len(tail) != 0 {
		t.Fatalf("unexpected buffered tail %q", tail)
	}
}

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
