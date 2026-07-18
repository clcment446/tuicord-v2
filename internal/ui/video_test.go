package ui

import (
	"strings"
	"testing"

	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
)

func hasGlyph(buf *screen.Buffer, glyph string) bool {
	for y := 0; y < buf.Height(); y++ {
		if strings.Contains(rowText(buf, y), glyph) {
			return true
		}
	}
	return false
}

func TestVideoAttachmentPlaceholderAndActivation(t *testing.T) {
	st := store.New(0)
	vurl := "https://cdn.discordapp.com/attachments/1/2/clip.mp4"
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Attachments: []store.Attachment{{URL: vurl, Filename: "clip.mp4", ContentType: "video/mp4", Size: 4096, W: 640, H: 360}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)

	var gotURL string
	var gotRegion media.Rect
	view.OnPlayVideo(func(u string, r media.Rect) { gotURL = u; gotRegion = r })

	buf := screen.NewBuffer(30, 12)
	view.Draw(buf.Clip(buf.Bounds()))

	if len(view.videoHits) != 1 {
		t.Fatalf("videoHits = %d, want 1 for a video attachment", len(view.videoHits))
	}
	h := view.videoHits[0]
	if h.url != vurl {
		t.Fatalf("video hit url = %q, want %q", h.url, vurl)
	}
	if h.cols <= 0 || h.rows <= 0 {
		t.Fatalf("video hit has empty region %+v", h)
	}
	if !hasGlyph(buf, "▶") {
		t.Fatal("no ▶ play glyph drawn for the video block")
	}

	// A click inside the block region starts playback with the block's cells as
	// the play region (origin 0,0 here, so local == absolute).
	if !view.activateAt(h.x, h.y, false) {
		t.Fatal("activateAt over the video block returned false")
	}
	if gotURL != vurl {
		t.Fatalf("play callback url = %q, want %q", gotURL, vurl)
	}
	if gotRegion.Cols != h.cols || gotRegion.Rows != h.rows || gotRegion.X != h.x || gotRegion.Y != h.y {
		t.Fatalf("play region = %+v, want x=%d y=%d cols=%d rows=%d", gotRegion, h.x, h.y, h.cols, h.rows)
	}
}

func TestVideoEmbedPosterRendersPlayGlyph(t *testing.T) {
	st := store.New(0)
	poster := "https://media.discordapp.net/external/abc/thumb.png"
	vurl := "https://cdn.discordapp.com/external/abc/clip.mp4"
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Embeds: []store.Embed{{Kind: store.EmbedVideo, ThumbURL: poster, VideoURL: vurl}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{poster: {img: solidTestImage(64, 36)}}

	buf := screen.NewBuffer(30, 12)
	view.Draw(buf.Clip(buf.Bounds()))

	if view.Animating() {
		t.Fatal("a video poster must not report as animating")
	}
	if len(view.videoHits) != 1 {
		t.Fatalf("videoHits = %d, want 1", len(view.videoHits))
	}
	if view.videoHits[0].url != vurl {
		t.Fatalf("video hit url = %q, want %q", view.videoHits[0].url, vurl)
	}
	if graphics := buf.Graphics(); len(graphics) != 1 {
		t.Fatalf("graphics = %d, want 1 poster placement", len(graphics))
	}
	if !hasGlyph(buf, "▶") {
		t.Fatal("no ▶ play glyph over the poster")
	}
}

func TestPlayingVideoBlanksPosterRegion(t *testing.T) {
	st := store.New(0)
	poster := "https://media.discordapp.net/external/abc/thumb.png"
	vurl := "https://cdn.discordapp.com/external/abc/clip.mp4"
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Embeds: []store.Embed{{Kind: store.EmbedVideo, ThumbURL: poster, VideoURL: vurl}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{poster: {img: solidTestImage(64, 36)}}

	buf := screen.NewBuffer(30, 12)
	view.Draw(buf.Clip(buf.Bounds())) // establishes layout for the play snapshot

	view.SetPlayingVideo(vurl)
	buf2 := screen.NewBuffer(30, 12)
	view.Draw(buf2.Clip(buf2.Bounds()))

	if view.playingVideo != vurl {
		t.Fatal("playback stopped unexpectedly on an unchanged layout")
	}
	if graphics := buf2.Graphics(); len(graphics) != 0 {
		t.Fatalf("graphics = %d while playing, want 0 (region blanked for mpv)", len(graphics))
	}
}

func TestPlayingVideoStopsOnResize(t *testing.T) {
	st := store.New(0)
	poster := "https://media.discordapp.net/external/abc/thumb.png"
	vurl := "https://cdn.discordapp.com/external/abc/clip.mp4"
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Embeds: []store.Embed{{Kind: store.EmbedVideo, ThumbURL: poster, VideoURL: vurl}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{poster: {img: solidTestImage(64, 36)}}

	stopped := false
	view.OnStopVideo(func() { stopped = true })

	wide := screen.NewBuffer(30, 12)
	view.Draw(wide.Clip(wide.Bounds()))
	view.SetPlayingVideo(vurl)

	// A resize moves the region; playback must stop so mpv does not keep drawing
	// into cells that shifted.
	narrow := screen.NewBuffer(20, 12)
	view.Draw(narrow.Clip(narrow.Bounds()))

	if !stopped {
		t.Fatal("resize did not stop inline playback")
	}
	if view.playingVideo != "" {
		t.Fatalf("playingVideo = %q after stop, want empty", view.playingVideo)
	}
}
