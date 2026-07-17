package ui

import (
	"image"
	"strings"
	"testing"

	"awesomeProject/internal/config"
	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
)

func TestFrameEmbedLinesPreservesAndOffsetsInteractionMetadata(t *testing.T) {
	header := &headerHit{key: "heading", level: 1}
	line := chatLine{
		segments: []chatSegment{{text: "hello"}},
		entities: []entityHit{{start: 1, end: 4, action: markup.Action{Kind: markup.ActionUserMention, Target: "7"}}},
		header:   header,
		spinner:  true,
	}

	framed := frameEmbedLines([]chatLine{line}, 5, screen.Style{}, screen.Style{})
	if len(framed) != 3 {
		t.Fatalf("framed lines = %d, want top/content/bottom", len(framed))
	}
	got := framed[1]
	if len(got.entities) != 1 || got.entities[0].start != 2 || got.entities[0].end != 5 {
		t.Fatalf("entity hits = %+v, want offset hit [2,5)", got.entities)
	}
	if got.header != header || !got.spinner {
		t.Fatalf("framed metadata = header %p spinner %t, want preserved", got.header, got.spinner)
	}
}

func TestEmbedFieldValueUsesDiscordMarkup(t *testing.T) {
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{})
	lines := view.renderEmbed(store.Message{}, store.Embed{Fields: []store.EmbedField{{Name: "status", Value: "**bold**"}}}, 0, 40, screen.Style{})
	for _, line := range lines {
		if strings.Contains(lineText(line), "bold") {
			for _, segment := range line.segments {
				if strings.Contains(segment.text, "bold") && segment.style.Attrs&screen.Bold != 0 {
					return
				}
			}
		}
	}
	t.Fatalf("embed field markup was not rendered bold: %+v", lines)
}

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

func TestEmbedHeaderRespectsColorOverride(t *testing.T) {
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{
		Overrides: &config.ColorOverrides{Rules: map[string]config.ColorRule{
			"messages.header1": {Fg: screen.RGB(1, 2, 3), HasFg: true},
		}},
	})
	lines := view.renderEmbed(store.Message{}, store.Embed{Title: "# Release"}, 0, 80, screen.Style{})
	for _, line := range lines {
		if strings.Contains(lineText(line), "Release") {
			for _, segment := range line.segments {
				if strings.Contains(segment.text, "Release") && segment.style.Fg == screen.RGB(1, 2, 3) {
					return
				}
			}
		}
	}
	t.Fatalf("embed header did not use the colors.conf override: %+v", lines)
}

func TestFocusedInlineMediaUsesFocusedStyle(t *testing.T) {
	view := NewChatView(store.New(0), func() store.ChannelID { return 1 }, nil, Styles{Cells: map[string]screen.Style{
		"messages.focused": {Fg: screen.RGB(1, 2, 3), Bg: screen.RGB(4, 5, 6)},
	}})
	buf := screen.NewBuffer(4, 1)
	view.drawInlineMedia(buf.Clip(buf.Bounds()), 0, 0, &inlineMedia{
		url: "https://cdn.test/image.png", cols: 2, rows: 1, img: image.NewRGBA(image.Rect(0, 0, 1, 1)),
		style: screen.Style{Fg: screen.RGB(7, 8, 9), Bg: screen.RGB(10, 11, 12)},
	}, 4, true)
	if got := buf.Cell(0, 0).Style; got.Fg != screen.RGB(1, 2, 3) || got.Bg != screen.RGB(4, 5, 6) || got.Attrs&screen.Reverse != 0 {
		t.Fatalf("focused image style = %+v, want configured focus colors without reversal", got)
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
