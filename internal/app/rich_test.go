package app

import (
	"encoding/json"
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
		Author:    discord.User{ID: 42, Username: "alice", Avatar: "avatarhash"},
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
	if got.AuthorAvatarURL != "https://cdn.discordapp.com/avatars/42/avatarhash.png" {
		t.Errorf("AuthorAvatarURL = %q", got.AuthorAvatarURL)
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

func TestMessageUpdatePreservesOmittedCachedFields(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.store.AppendMessage(store.Message{
		ID: 7, ChannelID: 3, Content: "keep", Flags: 42, Pinned: true,
		Attachments: []store.Attachment{{Filename: "keep.txt"}},
		Embeds:      []store.Embed{{Title: "old embed"}},
		Stickers:    []store.Sticker{{Name: "keep sticker"}},
		Components:  []store.Component{{Label: "keep component"}},
		ComponentTree: []store.ComponentNode{{
			Kind: store.ComponentTextDisplay, Content: "keep tree",
		}},
	})
	var event gateway.MessageUpdateEvent
	if err := json.Unmarshal([]byte(`{"id":"7","channel_id":"3","embeds":[]}`), &event); err != nil {
		t.Fatal(err)
	}

	a.handleMessageUpdate(&event)

	got := a.store.Messages(3)[0]
	if got.Content != "keep" || got.Flags != 42 || !got.Pinned || len(got.Attachments) != 1 ||
		len(got.Stickers) != 1 || len(got.Components) != 1 || len(got.ComponentTree) != 1 {
		t.Fatalf("omitted fields were overwritten: %+v", got)
	}
	if len(got.Embeds) != 0 {
		t.Fatalf("explicit embeds=[] did not clear embeds: %+v", got.Embeds)
	}
}

func TestMessageUpdateExplicitEmptyAndFalseClearCachedFields(t *testing.T) {
	a := newTestApp(&fakeSender{})
	a.store.AppendMessage(store.Message{
		ID: 8, ChannelID: 3, Content: "clear", Flags: 42, Pinned: true,
		Attachments:   []store.Attachment{{Filename: "clear.txt"}},
		Embeds:        []store.Embed{{Title: "clear embed"}},
		Stickers:      []store.Sticker{{Name: "clear sticker"}},
		Components:    []store.Component{{Label: "clear component"}},
		ComponentTree: []store.ComponentNode{{Kind: store.ComponentTextDisplay, Content: "clear tree"}},
	})
	var event gateway.MessageUpdateEvent
	if err := json.Unmarshal([]byte(`{
		"id":"8","channel_id":"3","content":"","flags":0,"pinned":false,
		"attachments":[],"embeds":[],"sticker_items":[],"components":[]
	}`), &event); err != nil {
		t.Fatal(err)
	}

	a.handleMessageUpdate(&event)

	got := a.store.Messages(3)[0]
	if got.Content != "" || got.Flags != 0 || got.Pinned || len(got.Attachments) != 0 ||
		len(got.Embeds) != 0 || len(got.Stickers) != 0 || len(got.Components) != 0 || len(got.ComponentTree) != 0 {
		t.Fatalf("explicit empty update did not clear cache: %+v", got)
	}
}

func TestMessageUpdatePatchesComponentsV2TreeForRenderer(t *testing.T) {
	// Arrange: the message was rendered with the original Components V2 tree.
	a := newTestApp(&fakeSender{})
	a.store.AppendMessage(store.Message{
		ID:        8,
		ChannelID: 3,
		Flags:     uint64(discord.IsComponentsV2),
		ComponentTree: []store.ComponentNode{{
			Kind:    store.ComponentTextDisplay,
			Content: "old content",
		}},
	})

	// Act: Discord sends the edited V2 message as a MESSAGE_UPDATE.
	a.handleMessageUpdate(&gateway.MessageUpdateEvent{Message: discord.Message{
		ID:        8,
		ChannelID: 3,
		Flags:     discord.IsComponentsV2,
		Components: discord.TopLevelComponents{
			&discord.ContainerComponent{Components: []discord.Component{
				&discord.TextDisplayComponent{Content: "new content"},
			}},
		},
	}})

	// Assert: the renderer's V2 tree must point at the edited content.
	msgs := a.store.Messages(3)
	if len(msgs) != 1 || len(msgs[0].ComponentTree) != 1 ||
		len(msgs[0].ComponentTree[0].Children) != 1 ||
		msgs[0].ComponentTree[0].Children[0].Content != "new content" {
		t.Fatalf("component tree after update = %+v", msgs)
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

func TestDiscordRoleGradientColorsDecodeFromJSON(t *testing.T) {
	var role discord.Role
	payload := []byte(`{"id":"7","name":"gradient","permissions":"0","position":1,"color":1122867,"colors":{"primary_color":1122867,"secondary_color":null,"tertiary_color":4478310}}`)
	if err := json.Unmarshal(payload, &role); err != nil {
		t.Fatalf("unmarshal role: %v", err)
	}
	if role.Colors.PrimaryColor != discord.Color(0x112233) || role.Colors.SecondaryColor != discord.NullColor || role.Colors.TertiaryColor != discord.Color(0x445566) {
		t.Fatalf("decoded role colors = %+v", role.Colors)
	}
}

func TestConvertRoleMapsGradientStops(t *testing.T) {
	role := convertRole(discord.Role{Colors: discord.RoleColors{
		PrimaryColor: 0x112233, SecondaryColor: 0x445566, TertiaryColor: 0x778899,
	}})
	if role.Colors != [3]uint32{0x112233, 0x445566, 0x778899} {
		t.Fatalf("gradient stops = %#v", role.Colors)
	}
}
