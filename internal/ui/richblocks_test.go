package ui

import (
	"strings"
	"testing"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
)

func TestChatViewColorsAuthorByRole(t *testing.T) {
	// Arrange: a member whose top colored role is red.
	st := store.New(0)
	st.UpsertChannel(store.Channel{ID: 1, GuildID: 5})
	st.UpsertRole(5, store.Role{ID: 10, Position: 2, Color: 0xff0000})
	st.UpsertMember(5, store.Member{ID: 42, Name: "alice", RoleIDs: []store.RoleID{10}})
	st.AppendMessage(store.Message{ChannelID: 1, AuthorID: 42, Author: "alice", Content: "hi"})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(20, 2)

	// Act
	view.Draw(buf.Clip(buf.Bounds()))

	// Assert: the author glyph is drawn in the role color.
	if got := buf.Cell(0, 0).Style.Fg; got != screen.RGB(0xff, 0x00, 0x00) {
		t.Errorf("author fg = %+v, want red", got)
	}
}

func TestChatViewRendersReactionsLine(t *testing.T) {
	// Arrange
	st := store.New(0)
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob", Content: "hi",
		Reactions: []store.Reaction{{EmojiName: "👍", Count: 3, Me: true}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(30, 3)

	// Act
	view.Draw(buf.Clip(buf.Bounds()))

	// Assert: reactions line appears with the arrow and count.
	found := false
	for y := range 3 {
		row := rowText(buf, y)
		if strings.Contains(row, "⤷") && strings.Contains(row, "3") {
			found = true
		}
	}
	if !found {
		t.Errorf("reactions line not found in %q/%q/%q", rowText(buf, 0), rowText(buf, 1), rowText(buf, 2))
	}
}

func TestChatViewRendersAttachmentChip(t *testing.T) {
	// Arrange
	st := store.New(0)
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Attachments: []store.Attachment{{Filename: "cat.png", ContentType: "image/png", Size: 2048}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(40, 3)

	// Act
	view.Draw(buf.Clip(buf.Bounds()))

	// Assert
	found := false
	for y := range 3 {
		if strings.Contains(rowText(buf, y), "cat.png") {
			found = true
		}
	}
	if !found {
		t.Error("attachment chip not rendered")
	}
}

func TestChatViewSuppressesSingleMediaURL(t *testing.T) {
	// Arrange: a lone gif link that Discord unfurled into a gifv embed.
	st := store.New(0)
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob", Content: "https://tenor.com/x.gif",
		Embeds: []store.Embed{{Kind: store.EmbedGIFV}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	buf := screen.NewBuffer(40, 4)

	// Act
	view.Draw(buf.Clip(buf.Bounds()))

	// Assert: the raw URL is not shown; the [GIF] chip is.
	var all strings.Builder
	for y := 0; y < 4; y++ {
		all.WriteString(rowText(buf, y))
		all.WriteByte('\n')
	}
	text := all.String()
	if strings.Contains(text, "tenor.com") {
		t.Errorf("raw URL should be suppressed, got %q", text)
	}
	if !strings.Contains(text, "[GIF]") {
		t.Errorf("expected [GIF] chip, got %q", text)
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
	}
	for _, c := range cases {
		if got := humanSize(c.in); got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
