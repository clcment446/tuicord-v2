package term

import "testing"

// TestSizeCellPixels pins the "unknown means unknown" contract: many terminals
// and tmux report zero pixel dimensions, and callers must fall back to a
// conventional cell size rather than divide by or trust a zero.
func TestSizeCellPixels(t *testing.T) {
	tests := []struct {
		name         string
		size         Size
		wantW, wantH int
		wantOK       bool
	}{
		{
			name:  "reported pixels divide into cells",
			size:  Size{Width: 80, Height: 24, XPixel: 800, YPixel: 480},
			wantW: 10, wantH: 20, wantOK: true,
		},
		{
			name:   "terminal reports no pixel size",
			size:   Size{Width: 80, Height: 24},
			wantOK: false,
		},
		{
			name:   "pixel size smaller than the cell grid is not usable",
			size:   Size{Width: 80, Height: 24, XPixel: 40, YPixel: 12},
			wantOK: false,
		},
		{
			name:   "zero cell grid does not divide by zero",
			size:   Size{XPixel: 800, YPixel: 480},
			wantOK: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, h, ok := tc.size.CellPixels()
			if ok != tc.wantOK {
				t.Fatalf("CellPixels ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && (w != tc.wantW || h != tc.wantH) {
				t.Errorf("CellPixels = %dx%d, want %dx%d", w, h, tc.wantW, tc.wantH)
			}
		})
	}
}
