package ui

import (
	"image"
	"testing"
	"time"

	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
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
