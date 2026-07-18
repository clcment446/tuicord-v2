package term

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"testing"
)

// TestReadClipboardImageRoundTrip is an integration test: it puts a PNG on the
// real system clipboard and reads it back through ReadClipboardImage. It needs
// a live clipboard (wl-copy/wl-paste or xclip) and is skipped unless
// TUICORD_CLIP_E2E=1, so it never runs in ordinary CI.
func TestReadClipboardImageRoundTrip(t *testing.T) {
	if os.Getenv("TUICORD_CLIP_E2E") != "1" {
		t.Skip("set TUICORD_CLIP_E2E=1 to run the live clipboard round-trip")
	}

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 60), G: uint8(y * 60), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	copier := exec.Command("wl-copy", "--type", "image/png")
	copier.Stdin = bytes.NewReader(buf.Bytes())
	if err := copier.Run(); err != nil {
		t.Fatalf("wl-copy: %v", err)
	}

	data, ext, err := ReadClipboardImage()
	if err != nil {
		t.Fatalf("ReadClipboardImage: %v", err)
	}
	if ext != "png" {
		t.Fatalf("ext = %q, want png", ext)
	}
	if !bytes.Equal(data, buf.Bytes()) {
		t.Fatalf("round-tripped %d bytes, want %d identical bytes", len(data), buf.Len())
	}
}
