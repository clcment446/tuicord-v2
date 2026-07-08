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

func TestConvertMessageComponentsV2Tree(t *testing.T) {
	msg := discord.Message{
		Flags: discord.IsComponentsV2,
		Components: discord.TopLevelComponents{
			&discord.ContainerComponent{
				AccentColor: discord.Color(0x5865F2),
				Components: []discord.Component{
					&discord.TextDisplayComponent{Content: "**Realm** progress"},
					&discord.SectionComponent{
						Components: []discord.Component{
							&discord.TextDisplayComponent{Content: "Choose your next action."},
						},
						Accessory: &discord.ButtonComponent{
							Label:    "Cultivate",
							CustomID: "cultivate",
							Style:    discord.PrimaryButtonStyle(),
						},
					},
					&discord.MediaGalleryComponent{Items: []discord.MediaGalleryComponentItem{
						{Media: discord.UnfurledMediaitem{URL: "https://example.test/a.png", Width: 100, Height: 80}, Description: "A"},
						{Media: discord.UnfurledMediaitem{URL: "https://example.test/b.png"}, Description: "B", Spoiler: true},
					}},
					&discord.SeparatorComponent{Divider: true, Spacing: discord.SeparatorComponentSpacingLargePadding},
					&discord.LabelComponent{
						Label:       "Battle note",
						Description: "Optional tactic",
						Component: &discord.TextInputComponent{
							CustomID:     "note",
							Placeholder:  "Type a tactic",
							Required:     true,
							LengthLimits: [2]int{1, 80},
						},
					},
					&discord.FileUploadComponent{
						CustomID:    "upload",
						Required:    true,
						ValueLimits: [2]int{0, 2},
						Values:      []discord.Snowflake{99},
					},
					&discord.RadioGroupComponent{
						CustomID: "stance",
						Value:    "guard",
						Options: []discord.RadioGroupComponentOption{
							{Label: "Strike", Value: "strike"},
							{Label: "Guard", Value: "guard", Default: true},
						},
					},
					&discord.CheckboxGroupComponent{
						CustomID:    "prep",
						ValueLimits: [2]int{0, 3},
						Values:      []string{"potion"},
						Options: []discord.CheckboxGroupComponentOption{
							{Label: "Potion", Value: "potion", Default: true},
							{Label: "Talisman", Value: "talisman"},
						},
					},
					&discord.CheckboxComponent{
						CustomID: "auto",
						Default:  true,
					},
				},
			},
		},
	}

	got := convertMessage(msg)
	if got.Flags&uint64(discord.IsComponentsV2) == 0 {
		t.Fatal("message flags did not preserve IsComponentsV2")
	}
	if len(got.ComponentTree) != 1 {
		t.Fatalf("ComponentTree = %+v, want one container", got.ComponentTree)
	}
	container := got.ComponentTree[0]
	if container.Kind != store.ComponentContainer || container.AccentColor != 0x5865F2 {
		t.Fatalf("container = %+v", container)
	}
	if len(container.Children) != 9 {
		t.Fatalf("container children = %d, want 9", len(container.Children))
	}
	if container.Children[0].Kind != store.ComponentTextDisplay || container.Children[0].Content != "**Realm** progress" {
		t.Fatalf("text child = %+v", container.Children[0])
	}
	section := container.Children[1]
	if section.Kind != store.ComponentSection || section.Accessory == nil || section.Accessory.CustomID != "cultivate" {
		t.Fatalf("section = %+v", section)
	}
	gallery := container.Children[2]
	if gallery.Kind != store.ComponentMediaGallery || len(gallery.Media) != 2 || !gallery.Media[1].Spoiler {
		t.Fatalf("gallery = %+v", gallery)
	}
	separator := container.Children[3]
	if separator.Kind != store.ComponentSeparator || !separator.Divider || separator.Spacing != 2 {
		t.Fatalf("separator = %+v", separator)
	}
	label := container.Children[4]
	if label.Kind != store.ComponentLabel || !label.InputField || len(label.Children) != 1 || label.Children[0].Kind != store.ComponentTextInput {
		t.Fatalf("label/text input = %+v", label)
	}
	fileUpload := container.Children[5]
	if fileUpload.Kind != store.ComponentFileUpload || fileUpload.CustomID != "upload" || fileUpload.MinValues != 0 || fileUpload.MaxValues != 2 || len(fileUpload.Values) != 1 {
		t.Fatalf("file upload = %+v", fileUpload)
	}
	radio := container.Children[6]
	if radio.Kind != store.ComponentRadioGroup || radio.Value != "guard" || len(radio.Options) != 2 || !radio.Options[1].Default {
		t.Fatalf("radio group = %+v", radio)
	}
	checkboxes := container.Children[7]
	if checkboxes.Kind != store.ComponentCheckboxGroup || len(checkboxes.Values) != 1 || len(checkboxes.Options) != 2 {
		t.Fatalf("checkbox group = %+v", checkboxes)
	}
	checkbox := container.Children[8]
	if checkbox.Kind != store.ComponentCheckbox || checkbox.Value != "true" {
		t.Fatalf("checkbox = %+v", checkbox)
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
