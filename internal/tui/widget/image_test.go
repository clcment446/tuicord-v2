package widget

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"testing"

	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

func TestImageDrawsASCIIFallback(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.Black)
	img.Set(1, 0, color.White)
	w := NewImageFrom(img)
	w.SetMode(ImageASCII)

	buf := screen.NewBuffer(2, 1)
	w.Draw(buf.Clip(buf.Bounds()))
	if got := bufferRow(buf, 0); got != " @" {
		t.Fatalf("ascii image = %q, want %q", got, " @")
	}
}

func TestImageANSIProtocols(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.Black)
	w := NewImageFrom(img)

	w.SetMode(ImageKitty)
	w.SetID(7)
	kitty, err := w.ANSI(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(kitty, []byte("\x1b_G")) {
		t.Fatalf("kitty output prefix = %q", string(kitty))
	}
	if !bytes.Contains(kitty, []byte("i=7")) || !bytes.Contains(kitty, []byte("p=7")) {
		t.Fatalf("kitty output missing stable id/placement: %q", string(kitty))
	}
	if !bytes.Contains(kitty, []byte("c=1,r=1")) {
		t.Fatalf("kitty output missing compatible cell placement: %q", string(kitty))
	}

	w.SetMode(ImageSixel)
	sixel, err := w.ANSI(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(sixel, []byte("\x1bPq")) {
		t.Fatalf("sixel output prefix = %q", string(sixel))
	}
}

func TestImageKittyANSISeparatesPixelsFromCells(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	w := NewImageFrom(img)
	w.SetID(9)

	kitty, err := w.KittyANSI(KittyOptions{
		PixelWidth:  24,
		PixelHeight: 16,
		CellWidth:   12,
		CellHeight:  8,
		X:           3,
		Y:           4,
		MoveCursor:  true,
		Z:           -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{
		[]byte("s=24"),
		[]byte("v=16"),
		[]byte("\x1b[5;4H"),
		[]byte("c=12"),
		[]byte("r=8"),
		[]byte("z=-1"),
	} {
		if !bytes.Contains(kitty, want) {
			t.Fatalf("kitty output missing %q: %q", string(want), string(kitty))
		}
	}
}

func TestImageDrawRegistersKittyGraphic(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	w := NewKittyImageFrom(img)
	w.SetID(11)

	buf := screen.NewBuffer(6, 4)
	w.Draw(buf.Clip(screen.Rect{X: 2, Y: 1, W: 3, H: 2}))

	graphics := buf.Graphics()
	if len(graphics) != 1 {
		t.Fatalf("graphics len = %d, want 1", len(graphics))
	}
	g := graphics[0]
	if g.Key != "kitty:11" {
		t.Fatalf("graphic key = %q, want kitty:11", g.Key)
	}
	for _, want := range [][]byte{
		[]byte("\x1b[2;3H"),
		[]byte("c=3"),
		[]byte("r=2"),
		[]byte("i=11"),
	} {
		if !bytes.Contains(g.Data, want) {
			t.Fatalf("kitty graphic missing %q: %q", string(want), string(g.Data))
		}
	}
	if !bytes.Contains(g.Upload, []byte("a=t")) {
		t.Fatalf("kitty upload = %q", string(g.Upload))
	}
	if !bytes.Contains(g.Clear, []byte("a=d,d=i,i=11,p=11")) {
		t.Fatalf("kitty clear = %q", string(g.Clear))
	}
	if !bytes.Contains(g.Free, []byte("a=d,d=I,i=11")) {
		t.Fatalf("kitty free = %q", string(g.Free))
	}
}

func TestImageKittyDoesNotDrawFallbackUnderTransparentPixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.SetRGBA(1, 1, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	w := NewKittyImageFrom(img)
	w.SetID(15)

	buf := screen.NewBuffer(4, 2)
	w.Draw(buf.Clip(buf.Bounds()))

	for y := 0; y < buf.Height(); y++ {
		for x := 0; x < buf.Width(); x++ {
			if got := buf.Cell(x, y).Content; got != " " {
				t.Fatalf("fallback cell %d,%d = %q, want blank under kitty image", x, y, got)
			}
		}
	}
	if graphics := buf.Graphics(); len(graphics) != 1 {
		t.Fatalf("graphics len = %d, want 1 kitty placement", len(graphics))
	}
}

func TestKittyUploadUsesStraightAlpha(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 128, G: 0, B: 0, A: 128})

	upload, err := kittyUpload(img, 1, 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	payload := kittyPayload(t, upload)
	if got, want := payload, []byte{255, 0, 0, 128}; !bytes.Equal(got, want) {
		t.Fatalf("payload = %v, want straight-alpha %v", got, want)
	}
}

func TestImageDrawClipsKittyGraphicWithoutSquashing(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 6))
	w := NewKittyImageFrom(img)
	w.SetID(13)

	buf := screen.NewBuffer(4, 2)
	w.Draw(buf.ClipWithin(
		screen.Rect{X: 0, Y: -1, W: 4, H: 3},
		screen.Rect{X: 0, Y: 0, W: 4, H: 2},
	))

	graphics := buf.Graphics()
	if len(graphics) != 1 {
		t.Fatalf("graphics len = %d, want 1", len(graphics))
	}
	g := graphics[0]
	for _, want := range [][]byte{
		[]byte("s=4"),
		[]byte("v=6"),
	} {
		if !bytes.Contains(g.Upload, want) {
			t.Fatalf("kitty upload missing full-size %q: %q", string(want), string(g.Upload))
		}
	}
	for _, want := range [][]byte{
		[]byte("\x1b[1;1H"),
		[]byte("x=0,y=2,w=4,h=4"),
		[]byte("c=4"),
		[]byte("r=2"),
	} {
		if !bytes.Contains(g.Data, want) {
			t.Fatalf("kitty placement missing clipped source %q: %q", string(want), string(g.Data))
		}
	}
	if g.Rect != (screen.Rect{X: 0, Y: 0, W: 4, H: 2}) {
		t.Fatalf("graphic rect = %+v, want visible clip", g.Rect)
	}
}

func kittyPayload(t *testing.T, upload []byte) []byte {
	t.Helper()
	start := bytes.IndexByte(upload, ';')
	if start < 0 {
		t.Fatalf("upload missing payload separator: %q", string(upload))
	}
	end := bytes.Index(upload[start+1:], []byte("\x1b\\"))
	if end < 0 {
		t.Fatalf("upload missing terminator: %q", string(upload))
	}
	raw := upload[start+1 : start+1+end]
	payload, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return payload
}

func TestImageFrameMovesKittyGraphicWithoutReuploading(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	w := NewKittyImageFrom(img)
	w.SetID(12)

	prev := screen.NewBuffer(8, 4)
	w.Draw(prev.Clip(screen.Rect{X: 1, Y: 1, W: 3, H: 2}))
	next := screen.NewBuffer(8, 4)
	w.Draw(next.Clip(screen.Rect{X: 2, Y: 1, W: 3, H: 2}))

	frame := screen.Frame(prev, next, false)
	if bytes.Contains(frame, []byte("a=t")) {
		t.Fatalf("move frame reuploaded image: %q", string(frame))
	}
	if bytes.Contains(frame, []byte("a=d")) {
		t.Fatalf("move frame deleted placement instead of replacing it: %q", string(frame))
	}
	if !bytes.Contains(frame, []byte("\x1b[2;3H")) || !bytes.Contains(frame, []byte("a=p")) {
		t.Fatalf("move frame missing placement: %q", string(frame))
	}
}

func TestImageFrameScrollsKittyGraphicWithoutReuploading(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 6))
	w := NewKittyImageFrom(img)
	w.SetID(14)

	prev := screen.NewBuffer(4, 3)
	w.Draw(prev.Clip(screen.Rect{X: 0, Y: 0, W: 4, H: 3}))
	next := screen.NewBuffer(4, 2)
	w.Draw(next.ClipWithin(
		screen.Rect{X: 0, Y: -1, W: 4, H: 3},
		screen.Rect{X: 0, Y: 0, W: 4, H: 2},
	))

	frame := screen.Frame(prev, next, false)
	if bytes.Contains(frame, []byte("a=t")) {
		t.Fatalf("scroll frame reuploaded image: %q", string(frame))
	}
	if !bytes.Contains(frame, []byte("x=0,y=2,w=4,h=4")) {
		t.Fatalf("scroll frame missing source crop: %q", string(frame))
	}
}

func TestKittyUploadCacheEvictsLeastRecentlyUsed(t *testing.T) {
	cache := newKittyUploadCache(5)
	if _, err := cache.Get("a", func() ([]byte, error) { return []byte("12"), nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Get("b", func() ([]byte, error) { return []byte("34"), nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Get("a", func() ([]byte, error) { return []byte("rebuilt"), nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Get("c", func() ([]byte, error) { return []byte("56"), nil }); err != nil {
		t.Fatal(err)
	}
	got, err := cache.Get("b", func() ([]byte, error) { return []byte("xy"), nil })
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "xy" {
		t.Fatalf("cache returned %q, want rebuilt value after LRU eviction", string(got))
	}
}

func TestImageSetImageReplacesSource(t *testing.T) {
	w := NewImageFrom(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	w.SetImage(image.NewRGBA(image.Rect(0, 0, 3, 2)))
	got := w.Measure(tui.Size{W: 20, H: 20})
	if got.W != 3 || got.H != 1 {
		t.Fatalf("Measure after SetImage = %+v, want 3x1", got)
	}
}
