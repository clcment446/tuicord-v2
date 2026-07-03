package media

import (
	"image"
	"testing"
)

// solidImage returns an opaque w×h image for downscale tests.
func solidImage(w, h int) image.Image {
	return image.NewRGBA(image.Rect(0, 0, w, h))
}

func TestDownscale_FitsWithinBudget(t *testing.T) {
	tests := []struct {
		name              string
		srcW, srcH        int
		maxCols, maxRows  int
		wantW, wantH      int
	}{
		{
			// Image already fits → returned unchanged (same pointer identity
			// is acceptable, but the bounds must match the source).
			name:    "image already fits",
			srcW:    10, srcH: 4,
			maxCols: 20, maxRows: 6,
			wantW: 10, wantH: 4,
		},
		{
			// 100×100 into 10 cols × 5 rows. Cell budget → 10 px wide, 10 px tall.
			// Limiting factor is height (100 → 10) → scale = 0.1.
			// Result: 10×10.
			name:    "square limited by height",
			srcW:    100, srcH: 100,
			maxCols: 10, maxRows: 5,
			wantW: 10, wantH: 10,
		},
		{
			// 200×50 into 10 cols × 10 rows.
			// Pixel budget: 10 wide, 20 tall.
			// Width scale = 10/200 = 0.05; height scale = 20/50 = 0.4.
			// Limiting factor is width → scale = 0.05.
			// Result: 10×2.
			name:    "wide image limited by width",
			srcW:    200, srcH: 50,
			maxCols: 10, maxRows: 10,
			wantW: 10, wantH: 2,
		},
		{
			// 1×100 tall image into 10 cols × 5 rows.
			// Pixel budget: 10 wide, 10 tall.
			// Width scale = 10/1 = 10; height scale = 10/100 = 0.1.
			// Limiting factor is height → scale = 0.1.
			// Result: 1×10.
			name:    "very tall narrow image",
			srcW:    1, srcH: 100,
			maxCols: 10, maxRows: 5,
			wantW: 1, wantH: 10,
		},
		{
			// 60×30 into 20 cols × 10 rows.
			// Pixel budget: 20 wide, 20 tall.
			// Width scale = 20/60 ≈ 0.333; height scale = 20/30 ≈ 0.667.
			// Limiting factor is width → scale ≈ 0.333.
			// Result: 20×10.
			name:    "landscape limited by width with cellAspect",
			srcW:    60, srcH: 30,
			maxCols: 20, maxRows: 10,
			wantW: 20, wantH: 10,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			img := solidImage(tc.srcW, tc.srcH)
			// Act.
			out := Downscale(img, tc.maxCols, tc.maxRows)
			// Assert.
			b := out.Bounds()
			if b.Dx() != tc.wantW || b.Dy() != tc.wantH {
				t.Errorf("Downscale(%d×%d, cols=%d, rows=%d) = %d×%d, want %d×%d",
					tc.srcW, tc.srcH, tc.maxCols, tc.maxRows,
					b.Dx(), b.Dy(), tc.wantW, tc.wantH)
			}
		})
	}
}

func TestDownscale_ZeroOrNegativeBudget(t *testing.T) {
	// Arrange: 50×50 image with degenerate budget.
	img := solidImage(50, 50)
	// Act: zero cols/rows become 1.
	out := Downscale(img, 0, 0)
	// Assert: must not panic and must return a non-empty image.
	b := out.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		t.Errorf("Downscale with zero budget returned empty image: %v", b)
	}
}

func TestDownscale_PreservesAspectRatio(t *testing.T) {
	// Arrange: 400×200 image (2:1 ratio) into a small budget.
	img := solidImage(400, 200)
	// Act.
	out := Downscale(img, 20, 5)
	b := out.Bounds()
	// Assert: width should be roughly twice the pixel height.
	// Pixel budget: 20 wide × 10 tall.
	// Width scale = 20/400 = 0.05; height scale = 10/200 = 0.05 (tie).
	// Result: 20×10.
	if b.Dx() <= 0 || b.Dy() <= 0 {
		t.Fatalf("Downscale returned empty bounds: %v", b)
	}
	ratio := float64(b.Dx()) / float64(b.Dy())
	// Original ratio in pixels is 2.0; allow 15% tolerance for integer rounding.
	const want = 2.0
	const tol = 0.15
	if ratio < want*(1-tol) || ratio > want*(1+tol) {
		t.Errorf("Downscale: aspect ratio = %.3f, want %.3f ± %.0f%%", ratio, want, tol*100)
	}
}
