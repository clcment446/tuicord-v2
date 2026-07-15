package media

import (
	"image"
	"image/color"
	"testing"
)

func benchSourceImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	return img
}

// BenchmarkDownscale1080pToBudget measures the cost this adds to the media
// fetch goroutine. It must stay well under what it saves on the UI goroutine in
// BenchmarkKittyUpload1920x1080, and it runs once per fetch rather than per frame.
func BenchmarkDownscale1080pToBudget(b *testing.B) {
	img := benchSourceImage(1920, 1080)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := Downscale(img, 2000, 120)
		if out.Bounds().Dy() > 240 {
			b.Fatalf("downscale height = %d, want <= 240", out.Bounds().Dy())
		}
	}
}
