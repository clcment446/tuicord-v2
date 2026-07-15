package widget

import (
	"image"
	"image/color"
	"testing"
)

// benchImage builds a non-uniform image so the base64 payload is realistic and
// the resampler cannot short-circuit on flat color.
func benchImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	return img
}

// benchKittyUpload calls kittyUpload directly, bypassing kittyCache, so the
// numbers reflect a cold encode — which is what every image's first draw pays
// on the UI goroutine inside Draw.
func benchKittyUpload(b *testing.B, srcW, srcH int) {
	b.Helper()
	img := benchImage(srcW, srcH)
	bounds := img.Bounds()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// ChatView.drawInlineMedia always passes the source bounds as the pixel
		// size (chatview.go:281), so mirror that here.
		out, err := kittyUpload(img, bounds.Dx(), bounds.Dy(), 1)
		if err != nil {
			b.Fatal(err)
		}
		if len(out) == 0 {
			b.Fatal("kittyUpload produced no bytes")
		}
	}
}

// BenchmarkKittyUpload1920x1080 is a full-resolution attachment as fetched
// today: no downscaling happens anywhere before this point.
func BenchmarkKittyUpload1920x1080(b *testing.B) { benchKittyUpload(b, 1920, 1080) }

// BenchmarkKittyUploadDownscaled is the same image after fetch-time downscaling
// to the height budget (MaxHeightCells=12 at ~20px/cell => 240px tall).
func BenchmarkKittyUploadDownscaled(b *testing.B) { benchKittyUpload(b, 427, 240) }
