package app

import (
	"testing"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

func TestConvertMessageMapsRichContent(t *testing.T) {
	// Arrange
	msg := discord.Message{
		ID:        7,
		ChannelID: 3,
		Author:    discord.User{ID: 42, Username: "alice"},
		Content:   "hi",
		Attachments: []discord.Attachment{{
			Filename: "cat.png", ContentType: "image/png",
			URL: "https://cdn/cat.png", Proxy: "https://proxy/cat.png",
			Width: 100, Height: 50, Size: 2048,
		}},
		Embeds: []discord.Embed{{
			Type: discord.GIFVEmbed, Title: "t", Description: "d",
			Video: &discord.EmbedVideo{Proxy: "https://proxy/vid.mp4"},
		}},
		Stickers: []discord.StickerItem{{ID: 9, Name: "wave", FormatType: discord.StickerFormatLottie}},
		Reactions: []discord.Reaction{{
			Count: 3, Me: true, Emoji: discord.Emoji{Name: "👍"},
		}},
		Components: discord.TopLevelComponents{
			&discord.ActionRowComponent{
				&discord.ButtonComponent{Label: "Click", CustomID: "cid", Style: discord.PrimaryButtonStyle()},
				&discord.ButtonComponent{Label: "Go", Style: discord.LinkButtonStyle("https://example.com")},
			},
		},
	}

	// Act
	got := convertMessage(msg)

	// Assert
	if got.AuthorID != 42 {
		t.Errorf("AuthorID = %d, want 42", got.AuthorID)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].ProxyURL != "https://proxy/cat.png" || got.Attachments[0].Size != 2048 {
		t.Errorf("attachments = %+v", got.Attachments)
	}
	if len(got.Embeds) != 1 || got.Embeds[0].Kind != store.EmbedGIFV || got.Embeds[0].VideoURL != "https://proxy/vid.mp4" {
		t.Errorf("embeds = %+v", got.Embeds)
	}
	if len(got.Stickers) != 1 || got.Stickers[0].Format != store.StickerLottie {
		t.Errorf("stickers = %+v", got.Stickers)
	}
	if len(got.Reactions) != 1 || got.Reactions[0].Count != 3 || !got.Reactions[0].Me {
		t.Errorf("reactions = %+v", got.Reactions)
	}
	if len(got.Components) != 2 {
		t.Fatalf("components = %+v, want 2", got.Components)
	}
	if got.Components[0].Kind != store.ComponentButton || got.Components[0].CustomID != "cid" {
		t.Errorf("button component = %+v", got.Components[0])
	}
	if got.Components[1].Kind != store.ComponentLinkButton || got.Components[1].URL != "https://example.com" {
		t.Errorf("link button component = %+v", got.Components[1])
	}
}

func TestMessageUpdatePatchesEmbedsInPlace(t *testing.T) {
	// Arrange
	a := newTestApp(&fakeSender{})
	a.store.AppendMessage(store.Message{ID: 7, ChannelID: 3, Author: "alice", Content: "look https://x.gg"})

	// Act: the unfurled embed arrives as an update.
	a.handleMessageUpdate(&gateway.MessageUpdateEvent{Message: discord.Message{
		ID: 7, ChannelID: 3, Content: "look https://x.gg",
		Embeds: []discord.Embed{{Type: discord.ImageEmbed, Title: "unfurled"}},
	}})

	// Assert
	msgs := a.store.Messages(3)
	if len(msgs) != 1 || len(msgs[0].Embeds) != 1 || msgs[0].Embeds[0].Title != "unfurled" {
		t.Fatalf("message after update = %+v", msgs)
	}
}

func TestReactionAddAndRemoveUpdateStore(t *testing.T) {
	// Arrange
	a := newTestApp(&fakeSender{})
	a.selfID = 99
	a.store.AppendMessage(store.Message{ID: 7, ChannelID: 3, Author: "alice"})

	// Act: someone reacts, then the current user reacts with the same emoji.
	a.handleReactionAdd(&gateway.MessageReactionAddEvent{
		ChannelID: 3, MessageID: 7, UserID: 1, Emoji: discord.Emoji{Name: "👍"},
	})
	a.handleReactionAdd(&gateway.MessageReactionAddEvent{
		ChannelID: 3, MessageID: 7, UserID: 99, Emoji: discord.Emoji{Name: "👍"},
	})

	// Assert: one reaction, count 2, Me set.
	got := a.store.Messages(3)[0].Reactions
	if len(got) != 1 || got[0].Count != 2 || !got[0].Me {
		t.Fatalf("reactions after adds = %+v", got)
	}

	// Act: the current user removes their reaction.
	a.handleReactionRemove(&gateway.MessageReactionRemoveEvent{
		ChannelID: 3, MessageID: 7, UserID: 99, Emoji: discord.Emoji{Name: "👍"},
	})

	// Assert: count 1, Me cleared.
	got = a.store.Messages(3)[0].Reactions
	if len(got) != 1 || got[0].Count != 1 || got[0].Me {
		t.Fatalf("reactions after remove = %+v", got)
	}

	// Act: remove-all clears the line.
	a.handleReactionRemoveAll(&gateway.MessageReactionRemoveAllEvent{ChannelID: 3, MessageID: 7})

	// Assert
	if got := a.store.Messages(3)[0].Reactions; len(got) != 0 {
		t.Fatalf("reactions after remove-all = %+v", got)
	}
}
