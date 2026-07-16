package ui

import (
	"testing"

	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
)

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
