package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

// convertChannelKind maps an arikawa channel type to a store.ChannelKind.
func convertChannelKind(t discord.ChannelType) store.ChannelKind {
	switch t {
	case discord.GuildVoice:
		return store.ChannelVoice
	case discord.GuildCategory:
		return store.ChannelCategory
	case discord.DirectMessage, discord.GroupDM:
		return store.ChannelDM
	default:
		return store.ChannelText
	}
}

// convertChannel maps an arikawa channel into a store.Channel.
func convertChannel(c discord.Channel) store.Channel {
	name := c.Name
	if name == "" && (c.Type == discord.DirectMessage || c.Type == discord.GroupDM) {
		name = dmName(c.DMRecipients)
	}
	return store.Channel{
		ID:       store.ChannelID(c.ID),
		GuildID:  store.GuildID(c.GuildID),
		Name:     name,
		Kind:     convertChannelKind(c.Type),
		Position: c.Position,
		ParentID: store.ChannelID(c.ParentID),
	}
}

func dmName(recipients []discord.User) string {
	if len(recipients) == 0 {
		return "Direct Message"
	}
	names := make([]string, 0, len(recipients))
	for _, user := range recipients {
		name := user.DisplayOrUsername()
		if name == "" {
			name = user.ID.String()
		}
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

// convertMessage maps an arikawa message into a store.Message, including its
// rich content: attachments, embeds, stickers, reactions, and V2 components.
func convertMessage(m discord.Message) store.Message {
	return store.Message{
		ID:            store.MessageID(m.ID),
		ChannelID:     store.ChannelID(m.ChannelID),
		AuthorID:      store.UserID(m.Author.ID),
		ApplicationID: uint64(m.ApplicationID),
		Author:        m.Author.DisplayOrUsername(),
		Content:       m.Content,
		Timestamp:     m.Timestamp.Time(),
		Nonce:         m.Nonce,
		Flags:         uint64(m.Flags),
		Attachments:   convertAttachments(m.Attachments),
		Embeds:        convertEmbeds(m.Embeds),
		Stickers:      convertStickers(m.Stickers),
		Reactions:     convertReactions(m.Reactions),
		Components:    convertComponents(m.Components),
		ComponentTree: convertComponentTree(
			m.Components,
			uint64(m.Flags)&uint64(discord.IsComponentsV2) != 0,
		),
		Pinned: m.Pinned,
	}
}

func convertAttachments(in []discord.Attachment) []store.Attachment {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.Attachment, len(in))
	for i, a := range in {
		out[i] = store.Attachment{
			URL:         string(a.URL),
			ProxyURL:    string(a.Proxy),
			Filename:    a.Filename,
			ContentType: a.ContentType,
			W:           int(a.Width),
			H:           int(a.Height),
			Size:        int64(a.Size),
		}
	}
	return out
}

func convertEmbeds(in []discord.Embed) []store.Embed {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.Embed, len(in))
	for i, e := range in {
		out[i] = convertEmbed(e)
	}
	return out
}

func convertEmbed(e discord.Embed) store.Embed {
	red, green, blue := e.Color.RGB()
	out := store.Embed{
		Kind:        convertEmbedKind(e.Type),
		Color:       uint32(red)<<16 | uint32(green)<<8 | uint32(blue),
		Title:       e.Title,
		URL:         string(e.URL),
		Description: e.Description,
		Fields:      convertEmbedFields(e.Fields),
	}
	if e.Author != nil {
		out.AuthorName = e.Author.Name
	}
	if e.Footer != nil {
		out.FooterText = e.Footer.Text
	}
	if e.Image != nil {
		out.ImageURL = proxyOr(string(e.Image.Proxy), string(e.Image.URL))
	}
	if e.Thumbnail != nil {
		out.ThumbURL = proxyOr(string(e.Thumbnail.Proxy), string(e.Thumbnail.URL))
	}
	if e.Video != nil {
		out.VideoURL = proxyOr(string(e.Video.Proxy), string(e.Video.URL))
	}
	if e.Provider != nil {
		out.Provider = e.Provider.Name
	}
	return out
}

func convertEmbedKind(t discord.EmbedType) store.EmbedKind {
	switch t {
	case discord.ImageEmbed:
		return store.EmbedImage
	case discord.VideoEmbed:
		return store.EmbedVideo
	case discord.GIFVEmbed:
		return store.EmbedGIFV
	case discord.LinkEmbed, discord.ArticleEmbed:
		return store.EmbedLink
	default:
		return store.EmbedRich
	}
}

func convertEmbedFields(in []discord.EmbedField) []store.EmbedField {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.EmbedField, len(in))
	for i, f := range in {
		out[i] = store.EmbedField{Name: f.Name, Value: f.Value, Inline: f.Inline}
	}
	return out
}

func convertStickers(in []discord.StickerItem) []store.Sticker {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.Sticker, len(in))
	for i, s := range in {
		out[i] = store.Sticker{
			ID:     uint64(s.ID),
			Name:   s.Name,
			Format: convertStickerFormat(s.FormatType),
		}
	}
	return out
}

func convertStickerFormat(f discord.StickerFormatType) store.StickerFormat {
	switch f {
	case discord.StickerFormatAPNG:
		return store.StickerAPNG
	case discord.StickerFormatLottie:
		return store.StickerLottie
	case 4: // GIF (added after arikawa's named constants)
		return store.StickerGIF
	default:
		return store.StickerPNG
	}
}

func convertReactions(in []discord.Reaction) []store.Reaction {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.Reaction, len(in))
	for i, r := range in {
		out[i] = store.Reaction{
			EmojiName: r.Emoji.Name,
			EmojiID:   uint64(r.Emoji.ID),
			Animated:  r.Emoji.Animated,
			Count:     r.Count,
			Me:        r.Me,
		}
	}
	return out
}

// convertComponents flattens the message's action rows into a flat button list.
// v1 renders buttons and link buttons; selects render as a disabled chip.
func convertComponents(rows discord.TopLevelComponents) []store.Component {
	var out []store.Component
	for _, row := range rows {
		action, ok := row.(*discord.ActionRowComponent)
		if !ok {
			continue
		}
		for _, c := range *action {
			if comp, ok := convertComponent(c); ok {
				out = append(out, comp)
			}
		}
	}
	return out
}

func convertComponent(c discord.Component) (store.Component, bool) {
	switch v := c.(type) {
	case *discord.ButtonComponent:
		style, url := buttonStyleURL(v)
		out := store.Component{
			Kind:     store.ComponentButton,
			Label:    v.Label,
			CustomID: string(v.CustomID),
			Style:    style,
			Disabled: v.Disabled,
		}
		if url != "" {
			out.Kind = store.ComponentLinkButton
			out.URL = url
		}
		return out, true
	case *discord.StringSelectComponent, *discord.UserSelectComponent,
		*discord.RoleSelectComponent, *discord.MentionableSelectComponent,
		*discord.ChannelSelectComponent:
		return store.Component{Kind: store.ComponentSelect, Disabled: true}, true
	default:
		return store.Component{}, false
	}
}

func convertComponentTree(rows discord.TopLevelComponents, v2 bool) []store.ComponentNode {
	if len(rows) == 0 {
		return nil
	}
	out := make([]store.ComponentNode, 0, len(rows))
	for _, row := range rows {
		node, ok := convertComponentNode(row)
		if !ok {
			continue
		}
		if !v2 && node.Kind == store.ComponentActionRow {
			out = append(out, node)
			continue
		}
		out = append(out, node)
	}
	return out
}

func convertComponentNode(c discord.Component) (store.ComponentNode, bool) {
	if c == nil {
		return store.ComponentNode{}, false
	}
	node := store.ComponentNode{RawType: int(c.Type())}
	switch v := c.(type) {
	case *discord.ActionRowComponent:
		node.Kind = store.ComponentActionRow
		for _, child := range *v {
			if converted, ok := convertComponentNode(child); ok {
				node.Children = append(node.Children, converted)
			}
		}
	case *discord.ButtonComponent:
		style, url := buttonStyleURL(v)
		node.Kind = store.ComponentButton
		node.Label = v.Label
		node.CustomID = string(v.CustomID)
		node.Style = style
		node.URL = url
		node.Disabled = v.Disabled
		if url != "" {
			node.Kind = store.ComponentLinkButton
		}
	case *discord.StringSelectComponent:
		node = convertSelectNode(node, string(v.CustomID), v.Placeholder, v.ValueLimits, v.Disabled)
		for _, opt := range v.Options {
			node.Options = append(node.Options, store.ComponentOption{
				Label:       opt.Label,
				Value:       opt.Value,
				Description: opt.Description,
				Default:     opt.Default,
			})
		}
	case *discord.UserSelectComponent:
		node = convertSelectNode(node, string(v.CustomID), v.Placeholder, v.ValueLimits, v.Disabled)
	case *discord.RoleSelectComponent:
		node = convertSelectNode(node, string(v.CustomID), v.Placeholder, v.ValueLimits, v.Disabled)
	case *discord.MentionableSelectComponent:
		node = convertSelectNode(node, string(v.CustomID), v.Placeholder, v.ValueLimits, v.Disabled)
	case *discord.ChannelSelectComponent:
		node = convertSelectNode(node, string(v.CustomID), v.Placeholder, v.ValueLimits, v.Disabled)
	case *discord.TextInputComponent:
		node.Kind = store.ComponentTextInput
		node.CustomID = string(v.CustomID)
		node.Label = v.Label
		node.Placeholder = v.Placeholder
		node.Value = v.Value
		node.Required = v.Required
		node.InputField = true
		node.MinValues = v.LengthLimits[0]
		node.MaxValues = v.LengthLimits[1]
	case *discord.ContainerComponent:
		node.Kind = store.ComponentContainer
		node.AccentColor = discordColor(v.AccentColor)
		node.Spoiler = v.Spoiler
		for _, child := range v.Components {
			if converted, ok := convertComponentNode(child); ok {
				node.Children = append(node.Children, converted)
			}
		}
	case *discord.SectionComponent:
		node.Kind = store.ComponentSection
		for _, child := range v.Components {
			if converted, ok := convertComponentNode(child); ok {
				node.Children = append(node.Children, converted)
			}
		}
		if accessory, ok := convertComponentNode(v.Accessory); ok {
			node.Accessory = &accessory
		}
	case *discord.TextDisplayComponent:
		node.Kind = store.ComponentTextDisplay
		node.Content = v.Content
	case *discord.ThumbnailComponent:
		node.Kind = store.ComponentThumbnail
		node.Media = []store.ComponentMedia{convertUnfurledMedia(v.Media, v.Description, v.Spoiler)}
	case *discord.MediaGalleryComponent:
		node.Kind = store.ComponentMediaGallery
		for _, item := range v.Items {
			node.Media = append(node.Media, convertUnfurledMedia(item.Media, item.Description, item.Spoiler))
		}
	case *discord.FileComponent:
		node.Kind = store.ComponentFile
		media := convertUnfurledMedia(v.File, "", v.Spoiler)
		media.Name = v.Name
		media.Size = int64(v.Size)
		node.Media = []store.ComponentMedia{media}
	case *discord.SeparatorComponent:
		node.Kind = store.ComponentSeparator
		node.Divider = v.Divider
		node.Spacing = int(v.Spacing)
	case *discord.LabelComponent:
		node.Kind = store.ComponentLabel
		node.Label = v.Label
		node.Description = v.Description
		if child, ok := convertComponentNode(v.Component); ok {
			node.Children = []store.ComponentNode{child}
			if child.InputField {
				node.InputField = true
			}
		}
	case *discord.FileUploadComponent:
		node.Kind = store.ComponentFileUpload
		node.CustomID = string(v.CustomID)
		node.InputField = true
		node.Required = v.Required
		node.MinValues = v.ValueLimits[0]
		node.MaxValues = v.ValueLimits[1]
		for _, value := range v.Values {
			node.Values = append(node.Values, value.String())
		}
	case *discord.RadioGroupComponent:
		node.Kind = store.ComponentRadioGroup
		node.CustomID = string(v.CustomID)
		node.InputField = true
		node.Required = v.Required
		node.Value = v.Value
		for _, opt := range v.Options {
			node.Options = append(node.Options, store.ComponentOption{
				Label:       opt.Label,
				Value:       opt.Value,
				Description: opt.Description,
				Default:     opt.Default,
			})
		}
	case *discord.CheckboxGroupComponent:
		node.Kind = store.ComponentCheckboxGroup
		node.CustomID = string(v.CustomID)
		node.InputField = true
		node.Required = v.Required
		node.Values = append(node.Values, v.Values...)
		node.MinValues = v.ValueLimits[0]
		node.MaxValues = v.ValueLimits[1]
		for _, opt := range v.Options {
			node.Options = append(node.Options, store.ComponentOption{
				Label:       opt.Label,
				Value:       opt.Value,
				Description: opt.Description,
				Default:     opt.Default,
			})
		}
	case *discord.CheckboxComponent:
		node.Kind = store.ComponentCheckbox
		node.CustomID = string(v.CustomID)
		node.InputField = true
		node.Required = true
		node.Value = fmt.Sprintf("%t", v.Value || v.Default)
	default:
		node.Kind = store.ComponentUnknown
	}
	return node, true
}

func convertSelectNode(node store.ComponentNode, customID, placeholder string, limits [2]int, disabled bool) store.ComponentNode {
	node.Kind = store.ComponentSelect
	node.CustomID = customID
	node.Placeholder = placeholder
	node.Disabled = disabled
	node.InputField = true
	node.MinValues = limits[0]
	node.MaxValues = limits[1]
	return node
}

func convertUnfurledMedia(in discord.UnfurledMediaitem, description string, spoiler bool) store.ComponentMedia {
	return store.ComponentMedia{
		URL:         in.URL,
		ProxyURL:    in.ProxyURL,
		Description: description,
		ContentType: in.ContentType,
		W:           in.Width,
		H:           in.Height,
		Spoiler:     spoiler,
	}
}

func discordColor(c discord.Color) uint32 {
	red, green, blue := c.RGB()
	return uint32(red)<<16 | uint32(green)<<8 | uint32(blue)
}

// buttonStyleURL extracts a button's style number and (for link buttons) its URL.
// arikawa hides both inside an unexported style type, so we round-trip through
// JSON — the marshaled form exposes "style" and "url".
func buttonStyleURL(b *discord.ButtonComponent) (style int, url string) {
	raw, err := json.Marshal(b)
	if err != nil {
		return 0, ""
	}
	var probe struct {
		Style int    `json:"style"`
		URL   string `json:"url"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		return 0, ""
	}
	return probe.Style, probe.URL
}

func proxyOr(proxy, direct string) string {
	if proxy != "" {
		return proxy
	}
	return direct
}

// convertMember maps an arikawa member into a store.Member.
func convertMember(m discord.Member) store.Member {
	name := m.Nick
	if name == "" {
		name = m.User.DisplayOrUsername()
	}
	roles := make([]store.RoleID, len(m.RoleIDs))
	for i, role := range m.RoleIDs {
		roles[i] = store.RoleID(role)
	}
	return store.Member{
		ID:      store.UserID(m.User.ID),
		Name:    name,
		RoleIDs: roles,
	}
}

func convertRole(r discord.Role) store.Role {
	red, green, blue := r.Color.RGB()
	color := uint32(red)<<16 | uint32(green)<<8 | uint32(blue)
	return store.Role{
		ID:          store.RoleID(r.ID),
		Name:        r.Name,
		Position:    r.Position,
		Color:       color,
		Hoist:       r.Hoist,
		Mentionable: r.Mentionable,
		Permissions: store.Permission(r.Permissions),
	}
}

// convertGuildFolders maps arikawa's user-settings guild folders into the
// store's representation, preserving order. Discord encodes an un-foldered
// guild as a single-element folder with an empty name and zero id, which
// store.OrderGuilds renders as a bare top-level guild.
func convertGuildFolders(in []gateway.GuildFolder) []store.GuildFolder {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.GuildFolder, 0, len(in))
	for _, f := range in {
		ids := make([]store.GuildID, 0, len(f.GuildIDs))
		for _, id := range f.GuildIDs {
			ids = append(ids, store.GuildID(id))
		}
		out = append(out, store.GuildFolder{
			ID:       int64(f.ID),
			Name:     f.Name,
			Color:    discordColor(f.Color),
			GuildIDs: ids,
		})
	}
	return out
}

// convertGuildEmojis maps a guild's custom emoji into the store catalog,
// skipping unicode entries (which have no snowflake ID) and unavailable emoji.
func convertGuildEmojis(in []discord.Emoji) []store.GuildEmoji {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.GuildEmoji, 0, len(in))
	for _, e := range in {
		if e.ID == 0 {
			continue
		}
		out = append(out, store.GuildEmoji{
			ID:       uint64(e.ID),
			Name:     e.Name,
			Animated: e.Animated,
		})
	}
	return out
}

// convertGuildStickers maps a guild's stickers into the store catalog.
func convertGuildStickers(in []discord.Sticker) []store.GuildSticker {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.GuildSticker, 0, len(in))
	for _, s := range in {
		if s.ID == 0 {
			continue
		}
		out = append(out, store.GuildSticker{
			ID:     uint64(s.ID),
			Name:   s.Name,
			Format: convertStickerFormat(s.FormatType),
		})
	}
	return out
}

// ingestGuild writes a guild and its channels/members into the store.
func ingestGuild(s *store.Store, g *gateway.GuildCreateEvent) {
	s.UpsertGuild(store.Guild{
		ID:      store.GuildID(g.ID),
		Name:    g.Name,
		OwnerID: store.UserID(g.OwnerID),
	})
	for _, r := range g.Roles {
		s.UpsertRole(store.GuildID(g.ID), convertRole(r))
	}
	for _, c := range g.Channels {
		c.GuildID = g.ID
		s.UpsertChannel(convertChannel(c))
	}
	for _, m := range g.Members {
		s.UpsertMember(store.GuildID(g.ID), convertMember(m))
	}
	s.SetGuildEmojis(store.GuildID(g.ID), convertGuildEmojis(g.Emojis))
	s.SetGuildStickers(store.GuildID(g.ID), convertGuildStickers(g.Stickers))
}

func ingestPrivateChannel(s *store.Store, c discord.Channel) {
	s.UpsertChannel(convertChannel(c))
}
