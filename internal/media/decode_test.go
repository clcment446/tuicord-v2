package media

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"testing"
	"time"
)

// makePNG returns the raw bytes of a minimal PNG with the given dimensions and
// solid fill colour. Used to exercise Decode without touching the network.
func makePNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("makePNG: encode: %v", err)
	}
	return buf.Bytes()
}

// makeGIF builds an in-memory GIF with nFrames frames, each delayCs centiseconds.
func makeGIF(t *testing.T, w, h, nFrames, delayCs int) []byte {
	t.Helper()
	palette := color.Palette{
		color.RGBA{R: 255, A: 255}, // index 0: red
		color.RGBA{G: 255, A: 255}, // index 1: green
		color.RGBA{B: 255, A: 255}, // index 2: blue
		color.RGBA{A: 255},         // index 3: black (background)
	}
	g := &gif.GIF{
		Config: image.Config{
			ColorModel: palette,
			Width:      w,
			Height:     h,
		},
		BackgroundIndex: 3,
	}
	for i := range nFrames {
		frame := image.NewPaletted(image.Rect(0, 0, w, h), palette)
		idx := byte(i % len(palette))
		for y := range h {
			for x := range w {
				frame.SetColorIndex(x, y, idx)
			}
		}
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, delayCs)
		g.Disposal = append(g.Disposal, 1) // do-not-dispose
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatalf("makeGIF: encode: %v", err)
	}
	return buf.Bytes()
}

func TestDecode_PNG(t *testing.T) {
	// Arrange: 4×3 red PNG.
	raw := makePNG(t, 4, 3, color.RGBA{R: 255, A: 255})
	// Act.
	img, err := Decode(bytes.NewReader(raw))
	// Assert.
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 4 || b.Dy() != 3 {
		t.Errorf("Decode: bounds = %v, want 4×3", b)
	}
}

func TestDecode_InvalidData(t *testing.T) {
	// Arrange.
	raw := []byte("this is not an image")
	// Act.
	_, err := Decode(bytes.NewReader(raw))
	// Assert.
	if err == nil {
		t.Fatal("Decode: expected error for invalid data, got nil")
	}
}

func TestDecodeGIF_FrameCount(t *testing.T) {
	tests := []struct {
		name    string
		nFrames int
	}{
		{"single frame", 1},
		{"two frames", 2},
		{"five frames", 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			raw := makeGIF(t, 8, 8, tc.nFrames, 10)
			// Act.
			frames, err := DecodeGIF(bytes.NewReader(raw))
			// Assert.
			if err != nil {
				t.Fatalf("DecodeGIF: unexpected error: %v", err)
			}
			if len(frames) != tc.nFrames {
				t.Errorf("DecodeGIF: got %d frames, want %d", len(frames), tc.nFrames)
			}
		})
	}
}

func TestDecodeGIF_Delay(t *testing.T) {
	// Arrange: 3 frames, each 5 centiseconds = 50 ms.
	raw := makeGIF(t, 4, 4, 3, 5)
	// Act.
	frames, err := DecodeGIF(bytes.NewReader(raw))
	// Assert.
	if err != nil {
		t.Fatalf("DecodeGIF: unexpected error: %v", err)
	}
	want := 50 * time.Millisecond
	for i, f := range frames {
		if f.Delay != want {
			t.Errorf("frame %d: Delay = %v, want %v", i, f.Delay, want)
		}
	}
}

func TestDecodeGIF_FramesBounds(t *testing.T) {
	// Arrange: 20×15 GIF with 2 frames.
	raw := makeGIF(t, 20, 15, 2, 10)
	// Act.
	frames, err := DecodeGIF(bytes.NewReader(raw))
	// Assert.
	if err != nil {
		t.Fatalf("DecodeGIF: unexpected error: %v", err)
	}
	for i, f := range frames {
		b := f.Image.Bounds()
		if b.Dx() != 20 || b.Dy() != 15 {
			t.Errorf("frame %d: bounds = %v, want 20×15", i, b)
		}
	}
}

func TestDecodeGIF_InvalidData(t *testing.T) {
	// Arrange.
	raw := []byte("not a gif")
	// Act.
	_, err := DecodeGIF(bytes.NewReader(raw))
	// Assert.
	if err == nil {
		t.Fatal("DecodeGIF: expected error for invalid data, got nil")
	}
}
