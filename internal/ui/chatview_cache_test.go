package ui

import (
	"strings"
	"testing"

	"awesomeProject/internal/markup"
	"awesomeProject/internal/store"
)

// renderText flattens a rendered line's segments so tests can assert on the
// text the cache produced without going through a screen buffer.
func renderText(lines []chatLine) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line.text)
		for _, seg := range line.segments {
			b.WriteString(seg.text)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func buttonMessage() store.Message {
	return store.Message{
		ID: 1, ChannelID: 1, Author: "bot", Content: "pick one",
		ComponentTree: []store.ComponentNode{{
			Kind: store.ComponentActionRow,
			Children: []store.ComponentNode{
				{Kind: store.ComponentButton, CustomID: "btn", Label: "Approve"},
			},
		}},
	}
}

// TestChatViewCacheInvalidatesOnComponentStateMutation is the regression test
// for the cache's whole design.
//
// SetComponentState patches ComponentTree in place, through the backing array
// that the store's shallow copies share. A cache that compared Message values
// to decide freshness would see "unchanged" and serve stale lines forever. Only
// the store revision distinguishes the versions.
func TestChatViewCacheInvalidatesOnComponentStateMutation(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(buttonMessage())
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})

	before := renderText(view.render(40))
	if !strings.Contains(before, "Approve") {
		t.Fatalf("first render = %q, want it to contain the button label", before)
	}
	if strings.Contains(before, "...") {
		t.Fatalf("first render = %q, want no pending marker yet", before)
	}

	if !st.SetComponentState(1, 1, "btn", store.ComponentStatePending) {
		t.Fatal("SetComponentState reported no match")
	}

	after := renderText(view.render(40))
	if !strings.Contains(after, "...") {
		t.Errorf("render after SetComponentState = %q, want the pending marker; "+
			"the cache served a stale body", after)
	}
}

// TestChatViewCacheInvalidatesOnReactionCountChange covers the other in-place
// patch: AddReaction increments Count through the shared Reactions array.
func TestChatViewCacheInvalidatesOnReactionCountChange(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID: 1, ChannelID: 1, Author: "alice", Content: "hi",
		Reactions: []store.Reaction{{EmojiName: "👍", Count: 1}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})

	if got := renderText(view.render(40)); !strings.Contains(got, "👍 1") {
		t.Fatalf("first render = %q, want the initial count", got)
	}

	st.AddReaction(1, 1, store.Reaction{EmojiName: "👍", Count: 1})

	if got := renderText(view.render(40)); !strings.Contains(got, "👍 2") {
		t.Errorf("render after AddReaction = %q, want the incremented count; "+
			"the cache served a stale body", got)
	}
}

// TestChatViewCacheInvalidatesWhenResolverGainsMembers pins the MetaRev
// dependency. The message never changes here — only the store state its
// mentions resolve against — so a per-message revision alone would not catch it.
func TestChatViewCacheInvalidatesWhenResolverGainsMembers(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "bob", Content: "hi <@42>"})
	resolver := func() markup.Resolver {
		return markup.Resolver{
			Member: func(id uint64) (string, bool) { return st.MemberName(1, store.UserID(id)) },
		}
	}
	view := NewChatView(st, func() store.ChannelID { return 1 }, resolver, Styles{})

	if got := renderText(view.render(40)); !strings.Contains(got, "@unknown-user") {
		t.Fatalf("first render = %q, want the mention unresolved", got)
	}

	st.UpsertMember(1, store.Member{ID: 42, Name: "alice"})

	if got := renderText(view.render(40)); !strings.Contains(got, "@alice") {
		t.Errorf("render after UpsertMember = %q, want the resolved mention; "+
			"the cache ignored a change to state the resolver reads", got)
	}
}

func TestChatViewCacheInvalidatesOnWidthChange(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID: 1, ChannelID: 1, Author: "alice",
		Content: "a message long enough that wrapping it narrower must produce more lines",
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})

	wide := len(view.render(80))
	narrow := len(view.render(20))
	if narrow <= wide {
		t.Errorf("render(20) = %d lines, render(80) = %d; the narrow render must wrap "+
			"more, so the cache must key on width", narrow, wide)
	}
}

// TestChatViewCacheInvalidatesOnComponentInteraction pins that this widget's own
// presentation state versions the cache: activating a control flashes it and
// must not be masked by a cache hit.
func TestChatViewCacheInvalidatesOnComponentInteraction(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(buttonMessage())
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.render(40)

	before := view.componentEpoch
	view.setComponentAction(componentAction{kind: store.ComponentButton, customID: "btn", label: "Approve"})
	if view.componentEpoch == before {
		t.Error("setComponentAction did not bump componentEpoch; cached bodies would " +
			"keep rendering the pre-interaction state")
	}
}

// TestChatViewEmojiPlacementKeysStableAcrossCachedRenders pins the invariant
// that makes per-message caching safe: placement keys are numbered from zero
// within each message, so a body's keys never depend on whether neighbouring
// messages were cache hits.
func TestChatViewEmojiPlacementKeysStableAcrossCachedRenders(t *testing.T) {
	st := store.New(0)
	for i := 1; i <= 3; i++ {
		st.AppendMessage(store.Message{
			ID: store.MessageID(i), ChannelID: 1, Author: "alice",
			Content: "<:a:111> and <:b:222>",
		})
	}
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})

	keysOf := func(lines []chatLine) []string {
		var out []string
		for _, line := range lines {
			for _, im := range line.inlineMedia {
				if im.media != nil {
					out = append(out, im.media.placementKey)
				}
			}
		}
		return out
	}

	first := keysOf(view.render(40))
	// The second render hits the cache for every message.
	second := keysOf(view.render(40))

	if len(first) != len(second) {
		t.Fatalf("placement key count changed across renders: %d then %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("placement key %d = %q on the cached render, want %q", i, second[i], first[i])
		}
	}
}

// TestChatViewCacheDoesNotCacheLoadingBodies pins that a body drawing a spinner
// re-renders every frame. The spinner advances via w.spinner, which is
// deliberately not part of the cache key — caching such a body would freeze it.
func TestChatViewCacheDoesNotCacheLoadingBodies(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID: 1, ChannelID: 1, Author: "alice",
		Attachments: []store.Attachment{{URL: "https://example.com/a.png", Filename: "a.png", ContentType: "image/png"}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	// Enable media without a fetcher so ensureMedia leaves a loading state
	// in place and no goroutine races the assertion.
	view.mediaCfg.Enabled = true
	view.media = map[string]*chatMediaState{
		"https://example.com/a.png": {loading: true},
	}

	view.render(40)
	if _, cached := view.bodyCache[messagePlacementPrefix(store.Message{ID: 1, ChannelID: 1})]; cached {
		t.Error("a body with loading media was cached; its spinner would freeze")
	}
}

// TestChatViewCacheHitsWhenNothingChanges is the performance claim as a test:
// a second render of unchanged state must not re-render any body.
func TestChatViewCacheHitsWhenNothingChanges(t *testing.T) {
	st := store.New(0)
	for i := 1; i <= 5; i++ {
		st.AppendMessage(store.Message{
			ID: store.MessageID(i), ChannelID: 1, Author: "alice", Content: "hello **world**",
		})
	}
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.render(40)

	for _, m := range st.Messages(1) {
		if _, ok := view.cachedBody(m, 1, 40); !ok {
			t.Fatalf("message %d missed the cache on an unchanged re-render", m.ID)
		}
	}
}

// TestChatViewCacheSeparatesChannels guards against a body rendered for one
// channel being served for another; renderThreadStarter reads the channel.
func TestChatViewCacheSeparatesChannels(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ID: 1, ChannelID: 1, Author: "alice", Content: "one"})
	st.AppendMessage(store.Message{ID: 2, ChannelID: 2, Author: "bob", Content: "two"})

	active := store.ChannelID(1)
	view := NewChatView(st, func() store.ChannelID { return active }, nil, Styles{})

	if got := renderText(view.render(40)); !strings.Contains(got, "one") {
		t.Fatalf("channel 1 render = %q, want its own message", got)
	}
	active = 2
	got := renderText(view.render(40))
	if !strings.Contains(got, "two") || strings.Contains(got, "one") {
		t.Errorf("channel 2 render = %q, want only its own message", got)
	}
}
