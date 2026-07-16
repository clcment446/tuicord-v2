package widget

import "testing"

func TestBottomScrollPreservesReadingPositionWhenContentGrows(t *testing.T) {
	var scroll BottomScroll
	scroll.Update(20, 4)
	scroll.SetOffset(3)

	scroll.Update(23, 4)

	if got, want := scroll.Offset(), 6; got != want {
		t.Fatalf("offset after content growth = %d, want %d", got, want)
	}
}

func TestBottomScrollPreservesOffsetWhenContentIsPrepended(t *testing.T) {
	var scroll BottomScroll
	scroll.Update(100, 10)
	scroll.SetOffset(20)
	scroll.UpdatePrepend(150, 10)

	if got := scroll.Offset(); got != 20 {
		t.Fatalf("offset after prepend = %d, want 20", got)
	}
}

func TestBottomScrollKeepsNewestPositionWhenContentGrows(t *testing.T) {
	var scroll BottomScroll
	scroll.Update(20, 4)

	scroll.Update(23, 4)

	if got, want := scroll.Offset(), 0; got != want {
		t.Fatalf("offset at newest position = %d, want %d", got, want)
	}
}

func TestBottomScrollClampsAfterContentShrinks(t *testing.T) {
	var scroll BottomScroll
	scroll.Update(20, 4)
	scroll.SetOffset(12)

	scroll.Update(8, 4)

	if got, want := scroll.Offset(), 4; got != want {
		t.Fatalf("offset after content shrink = %d, want %d", got, want)
	}
}

func TestBottomScrollClampsWhenContentFits(t *testing.T) {
	var scroll BottomScroll
	scroll.Update(20, 4)
	scroll.SetOffset(12)

	scroll.Update(3, 4)

	if got, want := scroll.Offset(), 0; got != want {
		t.Fatalf("offset when content fits = %d, want %d", got, want)
	}
}
