package ui

import (
	"strings"
	"testing"

	"awesomeProject/internal/markup"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
)

func rowText(buf *screen.Buffer, y int) string {
	var b strings.Builder
	for x := 0; x < buf.Width(); x++ {
		b.WriteString(buf.Cell(x, y).Content)
	}
	return strings.TrimRight(b.String(), " ")
}

func TestChatViewRendersBottomAligned(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ChannelID: 1, Author: "alice", Content: "first"})
	st.AppendMessage(store.Message{ChannelID: 1, Author: "bob", Content: "second"})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(20, 4)
	view.Draw(buf.Clip(buf.Bounds()))

	// 2 messages × (author + 1 content line) = 4 lines, fills the 4-row region.
	got := []string{rowText(buf, 0), rowText(buf, 1), rowText(buf, 2), rowText(buf, 3)}
	want := []string{"alice", "first", "bob", "second"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestChatViewMarksPendingAndFailed(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{ChannelID: 1, Author: "you", Content: "hi", Pending: true})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(30, 2)
	view.Draw(buf.Clip(buf.Bounds()))

	if !strings.Contains(rowText(buf, 0), "sending") {
		t.Errorf("pending header = %q, want to contain 'sending'", rowText(buf, 0))
	}
}

func TestChatViewResolvesMarkup(t *testing.T) {
	st := store.New(0)
	st.UpsertMember(1, store.Member{ID: 42, Name: "alice"})
	st.AppendMessage(store.Message{ChannelID: 1, Author: "bob", Content: "hi <@42> **bold**"})

	resolver := func() markup.Resolver {
		return markup.Resolver{
			Member: func(id uint64) (string, bool) { return st.MemberName(1, store.UserID(id)) },
		}
	}
	view := NewChatView(st, func() store.ChannelID { return 1 }, resolver, Styles{})
	buf := screen.NewBuffer(30, 2)
	view.Draw(buf.Clip(buf.Bounds()))

	// Mention resolved to @alice, bold delimiters stripped.
	if got := rowText(buf, 1); got != "hi @alice bold" {
		t.Errorf("content row = %q, want %q", got, "hi @alice bold")
	}
}

func TestChatViewEmptyChannel(t *testing.T) {
	st := store.New(0)
	view := NewChatView(st, func() store.ChannelID { return 99 }, nil, Styles{})
	buf := screen.NewBuffer(10, 3)
	// Should not panic and should render blank rows.
	view.Draw(buf.Clip(buf.Bounds()))
	if rowText(buf, 0) != "" {
		t.Errorf("empty channel row = %q, want blank", rowText(buf, 0))
	}
}
