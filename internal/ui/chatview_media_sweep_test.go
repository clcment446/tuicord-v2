package ui

import (
	"fmt"
	"image"
	"testing"

	"awesomeProject/internal/store"
)

// TestChatViewSweepsUnreferencedMedia pins that w.media stays bounded. It grew
// for the whole session before: every URL seen in every channel visited kept a
// decoded image alive.
func TestChatViewSweepsUnreferencedMedia(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "alice", Content: "hi"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.mediaCfg.Enabled = true
	view.media = map[string]*chatMediaState{}

	// Simulate a long session that accumulated media from channels no longer shown.
	for i := 0; i < maxMediaStates+50; i++ {
		view.media[string(rune('a'+i%26))+string(rune(i))] = &chatMediaState{}
	}

	view.render(40)

	if len(view.media) > maxMediaStates {
		t.Errorf("media map holds %d states after a render, want it swept to <= %d",
			len(view.media), maxMediaStates)
	}
}

// TestChatViewSweepKeepsInFlightMedia pins that a fetch still running is not
// evicted: its goroutine incremented the loading count and will post back into
// the state it expects to find.
func TestChatViewSweepKeepsInFlightMedia(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "alice", Content: "hi"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.mediaCfg.Enabled = true
	view.media = map[string]*chatMediaState{}

	for i := 0; i < maxMediaStates+50; i++ {
		view.media[string(rune('a'+i%26))+string(rune(i))] = &chatMediaState{}
	}
	view.media["https://example.com/in-flight.png"] = &chatMediaState{loading: true}

	view.render(40)

	if _, ok := view.media["https://example.com/in-flight.png"]; !ok {
		t.Error("the sweep evicted a state with a fetch still in flight")
	}
}

// TestChatViewCacheSurvivesMediaSweep guards the interaction between the two
// caches: a body still on screen must keep its media, or every frame would miss
// the body cache and re-render.
func TestChatViewCacheSurvivesMediaSweep(t *testing.T) {
	// Reference more media than the sweep budget from messages that render
	// every frame. That is what keeps the sweep running: media the current view
	// still uses cannot be evicted, so the map legitimately sits above the
	// budget and every render sweeps again.
	//
	// Two emoji per message, because the store caps history at
	// DefaultHistoryLimit — one per message could not exceed the budget.
	const messages = 150
	const perMessage = 2
	const referenced = messages * perMessage

	if referenced <= maxMediaStates {
		t.Fatalf("fixture references %d media, which is not above the %d budget; "+
			"the sweep would exit early and the test would prove nothing",
			referenced, maxMediaStates)
	}

	st := store.New(0)
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.mediaCfg.Enabled = true
	view.mediaCfg.EmojiImages = true
	view.media = map[string]*chatMediaState{}

	for i := 0; i < messages; i++ {
		content := ""
		for j := 0; j < perMessage; j++ {
			n := i*perMessage + j
			emojiID := uint64(1000 + n)
			content += fmt.Sprintf("<:e%d:%d> ", n, emojiID)
			// Pre-load each emoji as already fetched: ensureMedia only records a
			// dependency for a state that exists, and a loading body is never
			// cached at all.
			url := customEmojiURLParts(emojiID, fmt.Sprintf("e%d", n), false)
			view.media[url] = &chatMediaState{img: image.NewRGBA(image.Rect(0, 0, 48, 48))}
		}
		st.AppendMessage(store.Message{
			ID: store.MessageID(i + 1), ChannelID: 1, Author: "alice", Content: content,
		})
	}

	// Three passes are needed for the failure to surface: the first renders
	// cold and touches the media, the second serves cache hits, and only the
	// sweep following those hits can wrongly evict media the bodies still
	// depend on — which the third render then misses on.
	for i := 0; i < 3; i++ {
		view.render(40)
	}

	// Assert on the property directly: every one of these emoji is still on
	// screen, so none may be evicted. Counting cache misses would not catch
	// this — once the media is gone the body simply re-renders without it and
	// re-caches with no dependencies, which then "hits" while silently having
	// dropped the image and queued a refetch.
	if len(view.media) != referenced {
		t.Errorf("media map holds %d states after repeated renders, want all %d: the "+
			"sweep is evicting media that on-screen cached bodies still depend on, so "+
			"every frame re-renders and refetches", len(view.media), referenced)
	}
}
