package ui

import (
	"strings"
	"testing"

	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
)

func TestEmbedTitleUsesMarkupAndHeaderStyles(t *testing.T) {
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{Cells: map[string]screen.Style{
		"messages.header1": {Fg: screen.RGB(1, 2, 3)},
		"embeds.title":     {Fg: screen.RGB(4, 5, 6)},
	}})
	view.SetMedia(nil, media.Config{Enabled: false}, nil)
	lines := view.renderEmbed(store.Message{}, store.Embed{Title: "# <:wave:123>"}, 0, 80, screen.Style{})
	if len(lines) < 3 {
		t.Fatalf("embed lines = %+v, want framed title", lines)
	}
	found := false
	for _, line := range lines {
		if strings.Contains(lineText(line), ":wave:") {
			found = true
			if len(line.segments) < 2 || line.segments[1].style.Fg != screen.RGB(1, 2, 3) {
				t.Fatalf("title style = %+v, want heading override", line.segments)
			}
		}
	}
	if !found {
		t.Fatalf("embed title did not render custom emoji: %+v", lines)
	}
}

func lineText(line chatLine) string {
	var b strings.Builder
	for _, segment := range line.segments {
		b.WriteString(segment.text)
	}
	return b.String()
}

func TestEmbedMediaSpecFakeNitroMediaUsesNativeCellBudgets(t *testing.T) {
	emoji := embedMediaSpec(store.Embed{}, "https://cdn.discordapp.com/emojis/123.png?size=48", 80, 12)
	if emoji.maxCols != 2 || emoji.maxRows != 1 || !emoji.square {
		t.Fatalf("emoji spec = %+v, want 2x1 square", emoji)
	}
	sticker := embedMediaSpec(store.Embed{}, "https://media.discordapp.net/stickers/456.png?size=160", 80, 12)
	if sticker.maxRows != 8 || sticker.maxCols != 16 || !sticker.square {
		t.Fatalf("sticker spec = %+v, want 16x8 square", sticker)
	}
}

func TestRenderEmbedsSkipsMarkedFakeNitroMediaDuplicate(t *testing.T) {
	url := "https://media.discordapp.net/stickers/456.png"
	m := store.Message{
		Content: markup.FakeStickerLink("wave", url),
		Embeds:  []store.Embed{{Kind: store.EmbedImage, ImageURL: url}},
	}
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)

	if lines := view.renderEmbeds(m, 80, screen.Style{}); len(lines) != 0 {
		t.Fatalf("matching fake-Nitro embed rendered %d extra lines: %+v", len(lines), lines)
	}
}

func TestRenderEmbedsSkipsMatchingPrettyLinkEmbed(t *testing.T) {
	url := "https://cdn.discordapp.com/emojis/7.png?size=48&name=wave"
	m := store.Message{
		Content: markup.FakeEmojiLink("wave", url),
		Embeds: []store.Embed{{
			Kind:     store.EmbedLink,
			URL:      url,
			ImageURL: "https://media.discordapp.net/external/proxied-image.png",
		}},
	}
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)

	if lines := view.renderEmbeds(m, 80, screen.Style{}); len(lines) != 0 {
		t.Fatalf("matching pretty-link embed rendered %d extra lines: %+v", len(lines), lines)
	}
}

func TestRenderEmbedsKeepsDifferentMediaEmbed(t *testing.T) {
	m := store.Message{
		Content: markup.FakeEmojiLink("wave", "https://cdn.discordapp.com/emojis/7.png"),
		Embeds:  []store.Embed{{Kind: store.EmbedImage, ImageURL: "https://media.discordapp.net/stickers/456.png"}},
	}
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)

	if lines := view.renderEmbeds(m, 80, screen.Style{}); len(lines) == 0 {
		t.Fatal("unrelated media embed was incorrectly suppressed")
	}
}

func TestRenderEmbedsKeepsMarkerLookingExternalImage(t *testing.T) {
	url := "https://example.com/photo.png"
	m := store.Message{
		Content: markup.FakeEmojiLink("photo", url),
		Embeds:  []store.Embed{{Kind: store.EmbedImage, ImageURL: url}},
	}
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{})
	view.SetMedia(nil, media.DefaultConfig(), nil)

	if lines := view.renderEmbeds(m, 80, screen.Style{}); len(lines) == 0 {
		t.Fatal("marker-looking external image embed was incorrectly suppressed")
	}
}
