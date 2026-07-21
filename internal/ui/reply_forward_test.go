package ui

import (
	"strings"
	"testing"

	"awesomeProject/internal/markup"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
)

func TestChatViewRendersReplyReference(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID: 2, ChannelID: 1, AuthorID: 7, Author: "alice", Content: "sure!",
		Reply: &store.MessageReply{MessageID: 1, ChannelID: 1, AuthorID: 9, Author: "bob", Content: "can you do it?"},
	})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(40, 3)
	view.Draw(buf.Clip(buf.Bounds()))

	reply := rowText(buf, 1)
	if !strings.Contains(reply, "@bob") || !strings.Contains(reply, "can you do it?") {
		t.Errorf("reply line = %q, want author and preview", reply)
	}
	if got := rowText(buf, 2); got != "sure!" {
		t.Errorf("content line = %q, want %q", got, "sure!")
	}
}

func TestChatViewReplyPreviewResolvesMention(t *testing.T) {
	st := store.New(0)
	st.UpsertMember(1, store.Member{ID: 42, Name: "alice"})
	st.AppendMessage(store.Message{
		ID: 2, ChannelID: 1, AuthorID: 7, Author: "bob", Content: "yes",
		Reply: &store.MessageReply{MessageID: 1, ChannelID: 1, AuthorID: 9, Author: "carol", Content: "ask <@42> about it"},
	})

	resolver := func() markup.Resolver {
		return markup.Resolver{
			Member: func(id uint64) (string, bool) { return st.MemberName(1, store.UserID(id)) },
		}
	}
	view := NewChatView(st, func() store.ChannelID { return 1 }, resolver, Styles{})
	buf := screen.NewBuffer(48, 3)
	view.Draw(buf.Clip(buf.Bounds()))

	reply := rowText(buf, 1)
	if !strings.Contains(reply, "ask @alice about it") {
		t.Errorf("reply line = %q, want resolved mention", reply)
	}
	if strings.Contains(reply, "<@42>") {
		t.Errorf("reply line = %q, raw mention leaked into preview", reply)
	}
}

func TestChatViewRendersDeletedReplyReference(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID: 2, ChannelID: 1, AuthorID: 7, Author: "alice", Content: "sure!",
		Reply: &store.MessageReply{MessageID: 1, ChannelID: 1, Deleted: true},
	})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(48, 3)
	view.Draw(buf.Clip(buf.Bounds()))

	if !strings.Contains(rowText(buf, 1), "original message was deleted") {
		t.Errorf("reply line = %q, want deletion notice", rowText(buf, 1))
	}
}

func TestChatViewReplyPreviewTruncatesToWidth(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID: 2, ChannelID: 1, AuthorID: 7, Author: "alice", Content: "yes",
		Reply: &store.MessageReply{MessageID: 1, ChannelID: 1, AuthorID: 9, Author: "bob",
			Content: strings.Repeat("long words ", 20)},
	})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(30, 3)
	view.Draw(buf.Clip(buf.Bounds()))

	// The reply must stay a single line: author row, reply row, content row.
	if got := rowText(buf, 2); got != "yes" {
		t.Errorf("content line = %q, want %q (reply preview wrapped?)", got, "yes")
	}
	if !strings.Contains(rowText(buf, 1), "…") {
		t.Errorf("reply line = %q, want ellipsis", rowText(buf, 1))
	}
}

func TestChatViewRendersForwardedSnapshot(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID: 2, ChannelID: 1, AuthorID: 7, Author: "alice",
		Forwards: []store.ForwardedMessage{{Content: "the forwarded text"}},
	})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(48, 4)
	view.Draw(buf.Clip(buf.Bounds()))

	var all []string
	for y := 0; y < 4; y++ {
		all = append(all, rowText(buf, y))
	}
	joined := strings.Join(all, "\n")
	if !strings.Contains(joined, "↱ Forwarded") {
		t.Errorf("forward caption missing:\n%s", joined)
	}
	if !strings.Contains(joined, "the forwarded text") {
		t.Errorf("forwarded content missing:\n%s", joined)
	}
	if !strings.Contains(joined, "▍") {
		t.Errorf("forward quote bar missing:\n%s", joined)
	}
}

func TestChatViewRendersForwardedSnapshotEmbed(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ID: 2, ChannelID: 1, AuthorID: 7, Author: "alice",
		Forwards: []store.ForwardedMessage{{
			Embeds: []store.Embed{{Kind: store.EmbedRich, Title: "embedded title", Description: "embedded body"}},
		}},
	})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(48, 8)
	view.Draw(buf.Clip(buf.Bounds()))

	var all []string
	for y := 0; y < 8; y++ {
		all = append(all, rowText(buf, y))
	}
	joined := strings.Join(all, "\n")
	if !strings.Contains(joined, "embedded title") || !strings.Contains(joined, "embedded body") {
		t.Errorf("forwarded embed missing:\n%s", joined)
	}
}
