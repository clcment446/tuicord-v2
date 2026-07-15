package ui

import (
	"strings"
	"testing"

	"awesomeProject/internal/markup"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
)

// bufferText renders a whole screen buffer to text for comparison.
func bufferText(buf *screen.Buffer) string {
	var b strings.Builder
	for y := 0; y < buf.Height(); y++ {
		b.WriteString(rowText(buf, y))
		b.WriteString("\n")
	}
	return b.String()
}

// TestChatViewCachedOutputMatchesUncached is the end-to-end guarantee: caching
// must be invisible.
//
// It drives a realistic sequence of the mutations the gateway actually delivers
// against two views over one store. The "cold" view is rebuilt from scratch
// every step, so it never has a cache to hit; the "warm" view persists and
// therefore serves cached bodies. Any divergence in the drawn screen is a stale
// render — the exact class of bug the cache risks and that unit tests keyed on
// one input at a time can miss.
func TestChatViewCachedOutputMatchesUncached(t *testing.T) {
	st := store.New(0)
	resolver := func() markup.Resolver {
		return markup.Resolver{
			Member:  func(id uint64) (string, bool) { return st.MemberName(1, store.UserID(id)) },
			Channel: func(id uint64) (string, bool) { return st.ChannelName(store.ChannelID(id)) },
			Role: func(id uint64) (string, uint32, bool) {
				r, ok := st.Role(1, store.RoleID(id))
				if !ok {
					return "", 0, false
				}
				return r.Name, r.Color, true
			},
		}
	}
	newView := func() *ChatView {
		return NewChatView(st, func() store.ChannelID { return 1 }, resolver, Styles{})
	}

	warm := newView()
	draw := func(v *ChatView) string {
		buf := screen.NewBuffer(60, 40)
		v.Draw(buf.Clip(buf.Bounds()))
		return bufferText(buf)
	}

	steps := []struct {
		name  string
		apply func()
	}{
		{"seed history", func() {
			st.UpsertChannel(store.Channel{ID: 1, GuildID: 1, Name: "general"})
			st.AppendMessage(store.Message{
				ID: 1, ChannelID: 1, AuthorID: 7, Author: "alice",
				Content: "hello **world** with a mention <@42> and a channel <#1>",
			})
			st.AppendMessage(store.Message{
				ID: 2, ChannelID: 1, AuthorID: 8, Author: "bot",
				Content: "pick one",
				ComponentTree: []store.ComponentNode{{
					Kind: store.ComponentActionRow,
					Children: []store.ComponentNode{
						{Kind: store.ComponentButton, CustomID: "btn", Label: "Approve"},
					},
				}},
				Reactions: []store.Reaction{{EmojiName: "👍", Count: 1}},
			})
		}},
		{"a member arrives so the mention resolves", func() {
			st.UpsertMember(1, store.Member{ID: 42, Name: "alice"})
		}},
		{"a component interaction goes pending", func() {
			st.SetComponentState(1, 2, "btn", store.ComponentStatePending)
		}},
		{"a reaction is added in place", func() {
			st.AddReaction(1, 2, store.Reaction{EmojiName: "👍", Count: 1})
		}},
		{"a message is edited", func() {
			st.UpdateMessage(1, 1, func(m *store.Message) { m.Content = "edited *content* here" })
		}},
		{"an embed is unfurled onto an existing message", func() {
			st.UpdateMessage(1, 1, func(m *store.Message) {
				m.Embeds = []store.Embed{{
					Kind: store.EmbedRich, Title: "unfurled", Description: "a **description**",
				}}
			})
		}},
		{"a new message arrives", func() {
			st.AppendMessage(store.Message{ID: 3, ChannelID: 1, AuthorID: 7, Author: "alice", Content: "another"})
		}},
		{"the component resolves to success", func() {
			st.SetComponentState(1, 2, "btn", store.ComponentStateSuccess)
		}},
		{"a reaction is removed", func() {
			st.RemoveReaction(1, 2, "👍", 0, false)
		}},
		{"a message is pinned", func() {
			st.SetMessagePinned(1, 1, true)
		}},
		{"a role changes the author color", func() {
			st.UpsertRole(1, store.Role{ID: 3, Name: "mod", Color: 0xFF0000, Position: 1})
			st.UpsertMember(1, store.Member{ID: 7, Name: "alice", RoleIDs: []store.RoleID{3}})
		}},
	}

	for _, step := range steps {
		step.apply()
		// The cold view has no cache to serve from, so it is ground truth.
		want := draw(newView())
		got := draw(warm)
		if got != want {
			t.Errorf("after %q the cached view diverged from a cold render.\n"+
				"--- cached (got):\n%s\n--- cold (want):\n%s", step.name, got, want)
		}
	}
}

// TestChatViewCachedOutputMatchesUncachedAcrossWidths pins that a reflow after a
// terminal resize is not served a body wrapped for the old width.
func TestChatViewCachedOutputMatchesUncachedAcrossWidths(t *testing.T) {
	st := store.New(0)
	for i := 1; i <= 6; i++ {
		st.AppendMessage(store.Message{
			ID: store.MessageID(i), ChannelID: 1, AuthorID: 7, Author: "alice",
			Content: "a reasonably long message that will wrap differently at each width tested here",
		})
	}
	newView := func() *ChatView {
		return NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	}
	warm := newView()

	for _, width := range []int{80, 30, 55, 30, 100} {
		draw := func(v *ChatView) string {
			buf := screen.NewBuffer(width, 40)
			v.Draw(buf.Clip(buf.Bounds()))
			return bufferText(buf)
		}
		want := draw(newView())
		if got := draw(warm); got != want {
			t.Errorf("at width %d the cached view diverged from a cold render.\n"+
				"--- cached (got):\n%s\n--- cold (want):\n%s", width, got, want)
		}
	}
}
