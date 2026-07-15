package ui

import (
	"fmt"
	"testing"

	"awesomeProject/internal/markup"
	"awesomeProject/internal/store"
)

// benchWidth is a realistic chat column width: wide enough that markup wrapping
// does real work, narrow enough to force multi-line messages.
const benchWidth = 100

// benchMessage builds a message whose body exercises every renderer that
// ChatView.render drives: markup-heavy content, a rich embed, a component tree,
// and reactions. i varies the content so no two messages share a cache key.
func benchMessage(i int) store.Message {
	return store.Message{
		ID:        store.MessageID(i + 1),
		ChannelID: 1,
		AuthorID:  store.UserID(i%8 + 1),
		Author:    fmt.Sprintf("user%d", i%8),
		Content: fmt.Sprintf(
			"message %d with **bold** and *italic* and `code` and a mention <@42> "+
				"plus a channel <#1> and a ~~strike~~ and a https://example.com/%d link "+
				"that runs long enough to wrap across several rendered rows in the viewport",
			i, i),
		Embeds: []store.Embed{{
			Kind:        store.EmbedRich,
			Color:       0x5865F2,
			AuthorName:  "embed author",
			Title:       fmt.Sprintf("embed title %d", i),
			URL:         "https://example.com",
			Description: "embed **description** with markup that is long enough to wrap onto more than one line",
			Fields: []store.EmbedField{
				{Name: "field one", Value: "value **one**", Inline: true},
				{Name: "field two", Value: "value *two*", Inline: true},
			},
			FooterText: "footer text",
		}},
		ComponentTree: []store.ComponentNode{{
			Kind: store.ComponentActionRow,
			Children: []store.ComponentNode{
				{Kind: store.ComponentButton, CustomID: fmt.Sprintf("btn-a-%d", i), Label: "Approve", Style: 1},
				{Kind: store.ComponentButton, CustomID: fmt.Sprintf("btn-b-%d", i), Label: "Reject", Style: 4},
			},
		}},
		Reactions: []store.Reaction{
			{EmojiName: "👍", Count: 3},
			{EmojiName: "🎉", Count: 1, Me: true},
		},
	}
}

// benchStore fills a channel to the store's default history limit, which is the
// worst case ChatView.render faces in production.
func benchStore(tb testing.TB, n int) *store.Store {
	tb.Helper()
	st := store.New(0)
	st.UpsertMember(1, store.Member{ID: 42, Name: "alice"})
	for i := 0; i < n; i++ {
		st.AppendMessage(benchMessage(i))
	}
	return st
}

func benchView(st *store.Store) *ChatView {
	resolver := func() markup.Resolver {
		return markup.Resolver{
			Member: func(id uint64) (string, bool) { return st.MemberName(1, store.UserID(id)) },
		}
	}
	return NewChatView(st, func() store.ChannelID { return 1 }, resolver, Styles{})
}

// BenchmarkChatViewRender is the headline number: a cold render of a full
// history, which today runs on every single frame.
func BenchmarkChatViewRender(b *testing.B) {
	st := benchStore(b, store.DefaultHistoryLimit)
	view := benchView(st)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lines := view.render(benchWidth)
		if len(lines) == 0 {
			b.Fatal("render produced no lines")
		}
	}
}

// BenchmarkChatViewRenderCached measures a steady-state re-render with no
// intervening mutation — the case the render cache should collapse to ~0.
func BenchmarkChatViewRenderCached(b *testing.B) {
	st := benchStore(b, store.DefaultHistoryLimit)
	view := benchView(st)
	view.render(benchWidth) // warm
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lines := view.render(benchWidth)
		if len(lines) == 0 {
			b.Fatal("render produced no lines")
		}
	}
}

// BenchmarkChatViewRenderOneNewMessage is the realistic busy-channel case: a
// warm view re-rendered after a single message arrives. Only the new message
// should need rendering; today all of them do.
func BenchmarkChatViewRenderOneNewMessage(b *testing.B) {
	st := benchStore(b, store.DefaultHistoryLimit)
	view := benchView(st)
	view.render(benchWidth) // warm
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		st.AppendMessage(benchMessage(store.DefaultHistoryLimit + i))
		lines := view.render(benchWidth)
		if len(lines) == 0 {
			b.Fatal("render produced no lines")
		}
	}
}
