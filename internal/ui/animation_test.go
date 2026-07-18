package ui

import (
	"testing"
	"time"

	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
)

// gifFrames builds n distinct frames, each with the given per-frame delay.
func gifFrames(n int, delay time.Duration) []media.Frame {
	frames := make([]media.Frame, n)
	for i := range frames {
		// Distinct sizes give distinct image pointers and payloads per frame.
		frames[i] = media.Frame{Image: solidTestImage(4+i, 4+i), Delay: delay}
	}
	return frames
}

func TestAdvanceFramesLoops(t *testing.T) {
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{})
	frames := gifFrames(3, 100*time.Millisecond)
	state := &chatMediaState{img: frames[0].Image, frames: frames}

	start := time.Now()
	// First advance only primes lastTick; the frame must not move yet.
	if view.advanceFrames(state, start) {
		t.Fatal("first advance reported a frame change; it should only prime the clock")
	}
	if state.frameIdx != 0 {
		t.Fatalf("frameIdx = %d after priming, want 0", state.frameIdx)
	}

	// One frame's worth of time advances exactly one frame.
	if !view.advanceFrames(state, start.Add(100*time.Millisecond)) {
		t.Fatal("advance past the frame delay reported no change")
	}
	if state.frameIdx != 1 || state.img != frames[1].Image {
		t.Fatalf("frameIdx = %d, want 1 with img pointing at frame 1", state.frameIdx)
	}

	// Two frames' worth wraps from frame 1 → 2 → 0.
	if !view.advanceFrames(state, start.Add(300*time.Millisecond)) {
		t.Fatal("advance across two frames reported no change")
	}
	if state.frameIdx != 0 {
		t.Fatalf("frameIdx = %d after wrap, want 0", state.frameIdx)
	}
}

func TestAdvanceFramesResyncsAfterGap(t *testing.T) {
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{})
	frames := gifFrames(4, 40*time.Millisecond)
	state := &chatMediaState{img: frames[0].Image, frames: frames}

	start := time.Now()
	view.advanceFrames(state, start) // prime
	// A long gap (widget off-screen or app asleep) must not burst-advance frames.
	if view.advanceFrames(state, start.Add(5*time.Second)) {
		t.Fatal("a multi-second gap burst-advanced frames instead of resyncing")
	}
	if state.frameIdx != 0 {
		t.Fatalf("frameIdx = %d after gap, want 0 (resynced)", state.frameIdx)
	}
}

func TestChatViewAnimatesGIFAttachment(t *testing.T) {
	st := store.New(0)
	url := "https://cdn.discordapp.com/attachments/1/2/loop.gif"
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Attachments: []store.Attachment{{URL: url, Filename: "loop.gif", ContentType: "image/gif", Size: 4096}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	frames := gifFrames(2, 100*time.Millisecond)
	view.media = map[string]*chatMediaState{url: {img: frames[0].Image, frames: frames}}

	buf := screen.NewBuffer(16, 6)
	view.Draw(buf.Clip(buf.Bounds()))

	// A multi-frame GIF must place a graphic and mark itself as animating so the
	// runtime raises the tick cadence.
	if graphics := buf.Graphics(); len(graphics) != 1 {
		t.Fatalf("graphics len = %d, want 1 kitty placement for the GIF", len(graphics))
	}
	if !view.Animating() {
		t.Fatal("Animating() = false with a visible GIF, want true")
	}

	// The drawn frame follows the state's current frame index.
	first := frameUpload(buf)
	view.media[url].frameIdx = 1
	view.media[url].img = frames[1].Image
	buf2 := screen.NewBuffer(16, 6)
	view.Draw(buf2.Clip(buf2.Bounds()))
	if second := frameUpload(buf2); second == first {
		t.Fatal("advancing frameIdx did not change the uploaded frame payload")
	}
}

func TestChatViewStaticImageNotAnimating(t *testing.T) {
	st := store.New(0)
	url := "https://cdn.discordapp.com/attachments/1/2/cat.png"
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Attachments: []store.Attachment{{URL: url, Filename: "cat.png", ContentType: "image/png", Size: 2048}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{url: {img: solidTestImage(4, 4)}}

	buf := screen.NewBuffer(16, 6)
	view.Draw(buf.Clip(buf.Bounds()))
	if view.Animating() {
		t.Fatal("Animating() = true for a static image, want false")
	}
}

// frameUpload returns the upload payload of the first graphic, or nil.
func frameUpload(buf *screen.Buffer) string {
	g := buf.Graphics()
	if len(g) == 0 {
		return ""
	}
	return string(g[0].Upload)
}
