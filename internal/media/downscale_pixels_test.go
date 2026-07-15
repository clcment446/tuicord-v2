package media

import (
	"context"
	"image/color"
	"net/http"
	"testing"
)

func TestDownscaleToPixels_FitsWithinPixelBudget(t *testing.T) {
	tests := []struct {
		name         string
		srcW, srcH   int
		maxW, maxH   int
		wantW, wantH int
	}{
		{
			name: "image already fits is returned unchanged",
			srcW: 100, srcH: 50,
			maxW: 200, maxH: 100,
			wantW: 100, wantH: 50,
		},
		{
			// 1920×1080 into the real Kitty budget for a 12-row cap at 20px
			// cells: height is the limiting dimension (1080 → 240).
			name: "1080p into kitty height budget",
			srcW: 1920, srcH: 1080,
			maxW: 2000, maxH: 240,
			wantW: 426, wantH: 240,
		},
		{
			name: "width is the limiting dimension",
			srcW: 1000, srcH: 100,
			maxW: 100, maxH: 100,
			wantW: 100, wantH: 10,
		},
		{
			name: "non-positive budget degrades to a placeholder, not a panic",
			srcW: 100, srcH: 100,
			maxW: 0, maxH: 0,
			wantW: 1, wantH: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DownscaleToPixels(solidImage(tc.srcW, tc.srcH), tc.maxW, tc.maxH)
			b := got.Bounds()
			if b.Dx() != tc.wantW || b.Dy() != tc.wantH {
				t.Errorf("DownscaleToPixels bounds = %dx%d, want %dx%d",
					b.Dx(), b.Dy(), tc.wantW, tc.wantH)
			}
		})
	}
}

// TestDownscale_UsesHalfBlockCellAspect pins the distinction that makes
// DownscaleToPixels necessary: Downscale's cell budget assumes a half-block
// cell (1×2 px), which is wrong for pixel-addressed protocols like Kitty.
// Callers sizing a Kitty placement must go through DownscaleToPixels with a
// budget derived from the terminal's real cell size.
func TestDownscale_UsesHalfBlockCellAspect(t *testing.T) {
	got := Downscale(solidImage(1920, 1080), 60, 12)
	b := got.Bounds()
	// 60 cols → 60px, 12 rows → 24px. Height limits: 1080 → 24.
	if b.Dx() != 42 || b.Dy() != 24 {
		t.Errorf("Downscale bounds = %dx%d, want 42x24 (half-block geometry)", b.Dx(), b.Dy())
	}
}

func TestFetcher_Fetch_DownscalesToMaxPixels(t *testing.T) {
	raw := makePNG(t, 800, 600, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	f, _ := newStubFetcher(t, raw)
	f.MaxPixels.X, f.MaxPixels.Y = 200, 100

	img, err := f.Fetch(context.Background(), "https://example.com/big.png")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// 800×600 into 200×100: height limits → scale 1/6.
	if b := img.Bounds(); b.Dx() != 133 || b.Dy() != 100 {
		t.Errorf("Fetch bounds = %dx%d, want 133x100", b.Dx(), b.Dy())
	}
}

// TestFetcher_Fetch_DownscaledImageIsCached pins the pointer-stability property
// the Kitty upload cache depends on: it keys on the image pointer, so a warm
// LRU hit must return the same downscaled image rather than re-downscaling and
// minting a fresh pointer.
func TestFetcher_Fetch_DownscaledImageIsCached(t *testing.T) {
	raw := makePNG(t, 800, 600, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	stub := &stubDoer{status: http.StatusOK, body: raw}
	f := &Fetcher{HTTP: stub, Cache: newTempCache(t, 8)}
	f.MaxPixels.X, f.MaxPixels.Y = 200, 100
	url := "https://example.com/big.png"

	first, err := f.Fetch(context.Background(), url)
	if err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	second, err := f.Fetch(context.Background(), url)
	if err != nil {
		t.Fatalf("second Fetch: %v", err)
	}
	if first != second {
		t.Error("warm Fetch returned a different image pointer; the Kitty upload cache keys on it and would re-encode")
	}
	if b := second.Bounds(); b.Dy() != 100 {
		t.Errorf("cached image height = %d, want the downscaled 100", b.Dy())
	}
}

func TestFetcher_Fetch_ZeroMaxPixelsDisablesDownscaling(t *testing.T) {
	raw := makePNG(t, 800, 600, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	f, _ := newStubFetcher(t, raw)

	img, err := f.Fetch(context.Background(), "https://example.com/big.png")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 800 || b.Dy() != 600 {
		t.Errorf("Fetch bounds = %dx%d, want the untouched 800x600", b.Dx(), b.Dy())
	}
}

func TestConfigCellPixels_SubstitutesDefaults(t *testing.T) {
	if w, h := (Config{}).CellPixels(); w != defaultCellPixelWidth || h != defaultCellPixelHeight {
		t.Errorf("zero Config CellPixels = %dx%d, want %dx%d defaults",
			w, h, defaultCellPixelWidth, defaultCellPixelHeight)
	}
	if w, h := (Config{CellPixelWidth: 8, CellPixelHeight: 16}).CellPixels(); w != 8 || h != 16 {
		t.Errorf("CellPixels = %dx%d, want the reported 8x16", w, h)
	}
}
