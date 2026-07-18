package ui

import (
	"image"
	"testing"
	"time"

	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
)

func TestMediaViewerDrawsImage(t *testing.T) {
	v := newMediaViewer(Styles{}, "Esc to close", "u", solidTestImage(80, 60), nil, func() {})
	buf := screen.NewBuffer(40, 20)
	v.Draw(buf.Clip(buf.Bounds()))
	if g := buf.Graphics(); len(g) != 1 {
		t.Fatalf("viewer graphics = %d, want 1 centered image", len(g))
	}
}

func TestMediaViewerVideoModeDrawsNoGraphic(t *testing.T) {
	// Video mode has no still image; mpv paints the frames over the backdrop.
	v := newMediaViewer(Styles{}, "playing", "u", nil, nil, func() {})
	buf := screen.NewBuffer(40, 20)
	v.Draw(buf.Clip(buf.Bounds()))
	if g := buf.Graphics(); len(g) != 0 {
		t.Fatalf("video viewer graphics = %d, want 0", len(g))
	}
}

func TestVideoViewerMouseControlsDoNotClose(t *testing.T) {
	closed, toggled, replayed := false, false, false
	seek := -1.0
	v := newMediaViewer(Styles{}, "playing", "u", nil, nil, func() { closed = true })
	v.setVideoControls(
		func() { toggled = true },
		func() { replayed = true },
		func(percent float64) { seek = percent },
		nil,
	)
	buf := screen.NewBuffer(40, 10)
	v.Draw(buf.Clip(buf.Bounds()))

	if !v.Handle(input.MouseEvent{Kind: input.MousePress, X: 10, Y: 2}) || closed {
		t.Fatal("clicking the video body closed the overlay")
	}
	v.Handle(input.MouseEvent{Kind: input.MousePress, X: 1, Y: 8})
	v.Handle(input.MouseEvent{Kind: input.MousePress, X: 4, Y: 8})
	v.Handle(input.MouseEvent{Kind: input.MousePress, X: 23, Y: 8})
	if !toggled || !replayed || seek < 45 || seek > 55 {
		t.Fatalf("controls: toggled=%v replayed=%v seek=%v", toggled, replayed, seek)
	}
}

func TestVideoViewerConfigurableKeyboardControls(t *testing.T) {
	toggled, replayed := false, false
	var seeks []float64
	v := newMediaViewer(Styles{}, "playing", "u", nil, nil, func() {})
	v.setVideoControls(func() { toggled = true }, func() { replayed = true }, nil, nil)
	v.setVideoKeys("space", "left", "right", "r", func(seconds float64) { seeks = append(seeks, seconds) })

	for _, ev := range []input.KeyEvent{
		{Key: input.KeyRune, Rune: ' '},
		{Key: input.KeyLeft},
		{Key: input.KeyRight},
		{Key: input.KeyRune, Rune: 'r'},
	} {
		if !v.Handle(ev) {
			t.Fatalf("video key was not handled: %+v", ev)
		}
	}
	if !toggled || !replayed || len(seeks) != 2 || seeks[0] != -5 || seeks[1] != 5 {
		t.Fatalf("controls: toggled=%v replayed=%v seeks=%v", toggled, replayed, seeks)
	}
}

func TestVideoViewerReportsTerminalResizeOnce(t *testing.T) {
	v := newMediaViewer(Styles{}, "playing", "u", nil, nil, func() {})
	v.video = true
	v.width, v.height = 40, 10
	var gotW, gotH, calls int
	v.setVideoResize(func(w, h int) { gotW, gotH, calls = w, h, calls+1 })

	buf := screen.NewBuffer(60, 18)
	v.Draw(buf.Clip(buf.Bounds()))
	v.Draw(buf.Clip(buf.Bounds()))
	if calls != 1 || gotW != 60 || gotH != 18 {
		t.Fatalf("resize callback: calls=%d size=%dx%d", calls, gotW, gotH)
	}
}

func TestVideoRectForSizeReservesPaddingAndControls(t *testing.T) {
	got := videoRectForSize(80, 24)
	if got.X != 1 || got.Y != 1 || got.Cols != 78 || got.Rows != 20 {
		t.Fatalf("video rect = %+v, want inset 78x20 at 1,1", got)
	}
}

func TestMediaViewerEscCloses(t *testing.T) {
	closed := false
	v := newMediaViewer(Styles{}, "playing", "u", nil, nil, func() { closed = true })
	if !v.Handle(input.KeyEvent{Key: input.KeyEsc}) || !closed {
		t.Fatal("Esc did not close the media viewer")
	}
}

func TestMediaViewerAcceptsFullResolutionReplacement(t *testing.T) {
	v := newMediaViewer(Styles{}, "image", "u", solidTestImage(20, 10), nil, func() {})
	v.setImage(solidTestImage(200, 100))
	if got := v.img.Bounds().Dx(); got != 200 {
		t.Fatalf("replacement width = %d, want 200", got)
	}
}

func TestClickImageOpensViewer(t *testing.T) {
	st := store.New(0)
	url := "https://cdn.discordapp.com/attachments/1/2/pic.png"
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Attachments: []store.Attachment{{URL: url, Filename: "pic.png", ContentType: "image/png", Size: 2048, W: 80, H: 60}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{url: {img: solidTestImage(80, 60)}}

	var gotURL string
	var gotImg image.Image
	view.OnOpenMedia(func(u string, img image.Image, _ []media.Frame) { gotURL = u; gotImg = img })

	buf := screen.NewBuffer(40, 12)
	view.Draw(buf.Clip(buf.Bounds()))

	y, mediaX := -1, 0
	for i, line := range view.visibleLines {
		if line.media != nil && !line.media.video() && line.media.img != nil {
			y, mediaX = i, line.mediaX
			break
		}
	}
	if y < 0 {
		t.Fatal("no image media line was drawn")
	}
	if !view.activateAt(mediaX, y, false) {
		t.Fatal("clicking the image returned false")
	}
	if gotURL != url || gotImg == nil {
		t.Fatalf("onOpenMedia got url=%q img=%v, want the loaded image", gotURL, gotImg)
	}
}

func TestMediaViewerAnimatesGIFFrames(t *testing.T) {
	first := solidTestImage(8, 8)
	second := solidTestImage(9, 9)
	v := newMediaViewer(Styles{}, "gif", "u", first, []media.Frame{
		{Image: first, Delay: 50 * time.Millisecond},
		{Image: second, Delay: 50 * time.Millisecond},
	}, func() {})
	if !v.Animating() {
		t.Fatal("animated viewer did not request fast ticks")
	}
	start := time.Unix(100, 0)
	v.advance(start)
	if !v.advance(start.Add(50*time.Millisecond)) || v.img != second {
		t.Fatal("viewer did not advance to its second GIF frame")
	}
}
