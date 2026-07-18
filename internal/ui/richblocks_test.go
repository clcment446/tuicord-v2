package ui

import (
	"bytes"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
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

func TestChatViewOptInRoleGradientColorsAuthor(t *testing.T) {
	st := store.New(0)
	st.UpsertChannel(store.Channel{ID: 1, GuildID: 5})
	st.UpsertRole(5, store.Role{ID: 10, Position: 2, Colors: [3]uint32{0xff0000, 0x0000ff}})
	st.UpsertMember(5, store.Member{ID: 42, Name: "alice", RoleIDs: []store.RoleID{10}})
	st.AppendMessage(store.Message{ChannelID: 1, AuthorID: 42, Author: "alice", Content: "hi"})

	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetRoleGradients(true, false)
	buf := screen.NewBuffer(20, 2)
	view.Draw(buf.Clip(buf.Bounds()))

	if got := buf.Cell(0, 0).Style.Fg; got != screen.RGB(0xff, 0x00, 0x00) {
		t.Errorf("first author glyph fg = %+v, want red", got)
	}
	if got := buf.Cell(4, 0).Style.Fg; got == screen.RGB(0xff, 0x00, 0x00) {
		t.Errorf("last author glyph fg = %+v, want gradient color distinct from red", got)
	}
}

func TestChatViewRendersAuthorAvatarNextToGroupedName(t *testing.T) {
	const avatarURL = "https://cdn.discordapp.com/avatars/7/avatar.png"
	st := store.New(0)
	st.AppendMessage(store.Message{ChannelID: 1, AuthorID: 7, Author: "alice", AuthorAvatarURL: avatarURL, Content: "first"})
	st.AppendMessage(store.Message{ChannelID: 1, AuthorID: 7, Author: "alice", AuthorAvatarURL: avatarURL, Content: "second"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{avatarURL: {img: solidTestImage(48, 48)}}
	buf := screen.NewBuffer(24, 3)

	view.Draw(buf.Clip(buf.Bounds()))

	if got := rowText(buf, 0); got != "  alice" {
		t.Fatalf("author row = %q, want avatar slot followed by name", got)
	}
	if graphics := buf.Graphics(); len(graphics) != 1 || !bytes.Contains(graphics[0].Data, []byte("c=2")) || !bytes.Contains(graphics[0].Data, []byte("r=1")) {
		t.Fatalf("avatar graphics = %+v, want one 2x1 placement", graphics)
	}
}

func TestChatViewPrefersGuildProfileAvatar(t *testing.T) {
	const guildAvatarURL = "https://cdn.discordapp.com/guilds/5/users/7/avatars/server.png"
	st := store.New(0)
	st.UpsertChannel(store.Channel{ID: 1, GuildID: 5})
	st.UpsertMember(5, store.Member{ID: 7, AvatarURL: guildAvatarURL})
	st.AppendMessage(store.Message{ChannelID: 1, AuthorID: 7, Author: "alice", AuthorAvatarURL: "https://cdn.discordapp.com/avatars/7/global.png", Content: "hello"})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{guildAvatarURL: {img: solidTestImage(48, 48)}}
	buf := screen.NewBuffer(24, 2)

	view.Draw(buf.Clip(buf.Bounds()))

	if graphics := buf.Graphics(); len(graphics) != 1 {
		t.Fatalf("guild avatar graphics = %d, want 1", len(graphics))
	}
	if _, fetchedGlobal := view.media["https://cdn.discordapp.com/avatars/7/global.png"]; fetchedGlobal {
		t.Fatal("global avatar was fetched instead of the guild profile avatar")
	}
}

func TestChatViewAdvancesAnimatedGIFFrames(t *testing.T) {
	const gifURL = "https://cdn.example.test/spin.gif"
	first := solidTestImage(8, 8)
	second := image.NewRGBA(image.Rect(0, 0, 8, 8))
	st := store.New(0)
	st.AppendMessage(store.Message{ChannelID: 1, Author: "alice", Attachments: []store.Attachment{{URL: gifURL, Filename: "spin.gif", ContentType: "image/gif"}}})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{gifURL: {
		img:       first,
		frames:    []media.Frame{{Image: first, Delay: time.Millisecond}, {Image: second, Delay: time.Millisecond}},
		nextFrame: time.Now().Add(-time.Millisecond),
	}}
	buf := screen.NewBuffer(24, 4)
	view.Draw(buf.Clip(buf.Bounds()))

	if !view.Handle(input.TickEvent{}) {
		t.Fatal("animated GIF tick was not handled")
	}
	if got := view.media[gifURL].img; got != second {
		t.Fatal("animated GIF did not advance to its second frame")
	}
}

func TestChatViewShowsPausedGIFBadgeWhenAnimationDisabled(t *testing.T) {
	const gifURL = "https://cdn.example.test/paused.gif"
	img := solidTestImage(8, 8)
	st := store.New(0)
	st.AppendMessage(store.Message{ChannelID: 1, Author: "alice", Attachments: []store.Attachment{{URL: gifURL, Filename: "paused.gif", ContentType: "image/gif"}}})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.Config{Enabled: true, Animate: false, MaxHeightCells: 1}, nil)
	view.media = map[string]*chatMediaState{gifURL: {img: img}}
	buf := screen.NewBuffer(30, 3)
	view.Draw(buf.Clip(buf.Bounds()))

	if !strings.Contains(rowsText(buf), "[GIF]") {
		t.Fatalf("paused GIF badge missing:\n%s", rowsText(buf))
	}
}

func TestVideoAttachmentUsesPosterURL(t *testing.T) {
	attachment := store.Attachment{
		URL:      "https://cdn.discordapp.com/attachments/1/2/clip.mp4",
		ProxyURL: "https://media.discordapp.net/attachments/1/2/clip.mp4?width=640&height=360",
		Filename: "clip.mp4", ContentType: "video/mp4", W: 640, H: 360,
	}
	poster, ok := attachmentMediaURL(attachment)
	if !ok || !strings.Contains(poster, "format=jpeg") {
		t.Fatalf("video poster = %q, %t; want Discord JPEG proxy", poster, ok)
	}

	st := store.New(0)
	st.AppendMessage(store.Message{ChannelID: 1, Author: "alice", Attachments: []store.Attachment{attachment}})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.Config{Enabled: true, MaxHeightCells: 2}, nil)
	view.media = map[string]*chatMediaState{poster: {img: solidTestImage(640, 360)}}
	buf := screen.NewBuffer(24, 4)
	view.Draw(buf.Clip(buf.Bounds()))
	if graphics := buf.Graphics(); len(graphics) != 1 {
		t.Fatalf("video poster graphics = %d, want 1", len(graphics))
	}
	if !strings.Contains(rowsText(buf), "▶ clip.mp4") {
		t.Fatalf("video affordance missing:\n%s", rowsText(buf))
	}
}

func TestVideoAttachmentWithoutProxyFallsBackToChip(t *testing.T) {
	attachment := store.Attachment{URL: "https://cdn.discordapp.com/attachments/1/2/clip.mp4", ContentType: "video/mp4"}
	if poster, ok := attachmentMediaURL(attachment); ok || poster != "" {
		t.Fatalf("direct video poster = %q, %t; want no image fetch", poster, ok)
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

func TestChatViewEmbedsServerEmojiInReactions(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Reactions: []store.Reaction{{EmojiName: "party", EmojiID: 123, Count: 2}},
	})
	url := "https://cdn.discordapp.com/emojis/123.webp?size=48&name=party&lossless=true"
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{url: {img: solidTestImage(48, 48)}}
	buf := screen.NewBuffer(24, 3)

	view.Draw(buf.Clip(buf.Bounds()))

	for y := range buf.Height() {
		if got := rowText(buf, y); strings.Contains(got, ":party:") {
			t.Fatalf("reaction row contains colon markers: %q", got)
		}
	}
	graphics := buf.Graphics()
	if len(graphics) != 1 {
		t.Fatalf("graphics len = %d, want 1 embedded reaction emoji", len(graphics))
	}
	if !bytes.Contains(graphics[0].Data, []byte("c=2")) || !bytes.Contains(graphics[0].Data, []byte("r=1")) {
		t.Fatalf("reaction emoji placement = %q, want 2 columns by 1 row", graphics[0].Data)
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

func TestChatViewRendersLoadedAttachmentImage(t *testing.T) {
	st := store.New(0)
	url := "https://cdn.discordapp.com/attachments/1/2/cat.png"
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Attachments: []store.Attachment{{URL: url, Filename: "cat.png", ContentType: "image/png", Size: 2048}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{url: {img: solidTestImage(4, 4)}}
	buf := screen.NewBuffer(16, 4)

	view.Draw(buf.Clip(buf.Bounds()))

	for y := 1; y < buf.Height(); y++ {
		for x := 0; x < buf.Width(); x++ {
			if got := buf.Cell(x, y).Content; got != " " {
				t.Fatalf("loaded attachment fallback cell %d,%d = %q, want blank under kitty image", x, y, got)
			}
		}
	}
	if graphics := buf.Graphics(); len(graphics) != 1 {
		t.Fatalf("graphics len = %d, want 1 kitty placement", len(graphics))
	}
}

func TestChatViewSizesAttachmentMediaFromMetadata(t *testing.T) {
	st := store.New(0)
	url := "https://cdn.discordapp.com/attachments/1/2/cat.png"
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Attachments: []store.Attachment{{URL: url, Filename: "cat.png", ContentType: "image/png", W: 400, H: 300}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{url: {img: solidTestImage(400, 300)}}
	buf := screen.NewBuffer(80, 14)

	view.Draw(buf.Clip(buf.Bounds()))

	graphics := buf.Graphics()
	if len(graphics) != 1 {
		t.Fatalf("graphics len = %d, want 1", len(graphics))
	}
	for _, want := range [][]byte{
		[]byte("s=400"),
		[]byte("v=300"),
	} {
		if !bytes.Contains(graphics[0].Upload, want) {
			t.Fatalf("upload missing %q: %q", string(want), string(graphics[0].Upload))
		}
	}
	for _, want := range [][]byte{
		[]byte("c=32"),
		[]byte("r=12"),
	} {
		if !bytes.Contains(graphics[0].Data, want) {
			t.Fatalf("placement missing %q: %q", string(want), string(graphics[0].Data))
		}
	}
}

func TestChatViewRespectsMediaProxyQueryDimensions(t *testing.T) {
	st := store.New(0)
	url := "https://media.discordapp.net/attachments/1/2/cat.png?width=400&height=200"
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Attachments: []store.Attachment{{ProxyURL: url, Filename: "cat.png", ContentType: "image/png", W: 800, H: 800}},
	})
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{url: {img: solidTestImage(800, 800)}}
	buf := screen.NewBuffer(80, 14)

	view.Draw(buf.Clip(buf.Bounds()))

	graphics := buf.Graphics()
	if len(graphics) != 1 {
		t.Fatalf("graphics len = %d, want 1", len(graphics))
	}
	for _, want := range [][]byte{
		[]byte("s=800"),
		[]byte("v=800"),
	} {
		if !bytes.Contains(graphics[0].Upload, want) {
			t.Fatalf("upload missing original-size %q: %q", string(want), string(graphics[0].Upload))
		}
	}
	for _, want := range [][]byte{
		[]byte("c=48"),
		[]byte("r=12"),
	} {
		if !bytes.Contains(graphics[0].Data, want) {
			t.Fatalf("placement missing query-sized %q: %q", string(want), string(graphics[0].Data))
		}
	}
}

func TestChatViewRendersStickerInSquareMediaBox(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob",
		Stickers: []store.Sticker{{ID: 9, Name: "wave", Format: store.StickerPNG}},
	})
	url := "https://media.discordapp.net/stickers/9.png?size=160"
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{url: {img: solidTestImage(320, 160)}}
	buf := screen.NewBuffer(40, 10)

	view.Draw(buf.Clip(buf.Bounds()))

	graphics := buf.Graphics()
	if len(graphics) != 1 {
		t.Fatalf("graphics len = %d, want 1", len(graphics))
	}
	for _, want := range [][]byte{
		[]byte("c=16"),
		[]byte("r=8"),
	} {
		if !bytes.Contains(graphics[0].Data, want) {
			t.Fatalf("sticker placement missing square box %q: %q", string(want), string(graphics[0].Data))
		}
	}
}

func TestChatViewRendersInlineServerEmoji(t *testing.T) {
	st := store.New(0)
	st.AppendMessage(store.Message{
		ChannelID: 1, Author: "bob", Content: "hi <:party:123> ok",
	})
	url := "https://cdn.discordapp.com/emojis/123.webp?size=48&name=party&lossless=true"
	view := NewChatView(st, func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)
	view.media = map[string]*chatMediaState{url: {img: solidTestImage(48, 48)}}
	buf := screen.NewBuffer(24, 3)

	view.Draw(buf.Clip(buf.Bounds()))

	if got := rowText(buf, 1); !strings.Contains(got, "hi    ok") {
		t.Fatalf("message row = %q, want inline emoji slot between text", got)
	}
	graphics := buf.Graphics()
	if len(graphics) != 1 {
		t.Fatalf("graphics len = %d, want 1 inline emoji placement", len(graphics))
	}
	for _, want := range [][]byte{
		[]byte("s=48"),
		[]byte("v=48"),
	} {
		if !bytes.Contains(graphics[0].Upload, want) {
			t.Fatalf("emoji upload missing original-size %q: %q", string(want), string(graphics[0].Upload))
		}
	}
	for _, want := range [][]byte{
		[]byte("c=2"),
		[]byte("r=1"),
	} {
		if !bytes.Contains(graphics[0].Data, want) {
			t.Fatalf("emoji placement missing %q: %q", string(want), string(graphics[0].Data))
		}
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
	if strings.ContainsAny(text, "╭╰│") {
		t.Errorf("pure media embed should not be framed, got %q", text)
	}
}

func solidTestImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 40, B: 80, A: 255})
		}
	}
	return img
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
