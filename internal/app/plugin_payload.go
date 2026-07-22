package app

import (
	"encoding/json"

	"github.com/diamondburned/arikawa/v3/discord"
)

// pluginMessagePayload keeps the plugin boundary independent of arikawa's
// component interfaces. In particular, Lua plugins need the live custom IDs
// and select options to automate component-driven bots.
func pluginMessagePayload(message discord.Message, guildID uint64, bot bool) map[string]any {
	return map[string]any{
		"id":         uint64(message.ID),
		"channel_id": uint64(message.ChannelID),
		"guild_id":   guildID,
		"author_id":  uint64(message.Author.ID),
		"author":     message.Author.Username,
		"content":    message.Content,
		"bot":        bot,
		"components": pluginComponents(message.Components),
		"embeds":     pluginEmbeds(message.Embeds),
	}
}

func pluginEmbeds(embeds []discord.Embed) []any {
	out := make([]any, 0, len(embeds))
	for _, embed := range embeds {
		fields := make([]any, 0, len(embed.Fields))
		for _, field := range embed.Fields {
			fields = append(fields, map[string]any{"name": field.Name, "value": field.Value})
		}
		out = append(out, map[string]any{
			"title":       embed.Title,
			"description": embed.Description,
			"fields":      fields,
		})
	}
	return out
}

func pluginComponents(components discord.TopLevelComponents) []any {
	out := make([]any, 0, len(components))
	for _, component := range components {
		if value := pluginComponent(component); value != nil {
			out = append(out, value)
		}
	}
	return out
}

func pluginComponent(component discord.Component) map[string]any {
	if component == nil {
		return nil
	}
	base := map[string]any{"type": int(component.Type())}
	switch value := component.(type) {
	case *discord.ButtonComponent:
		base["label"] = value.Label
		base["custom_id"] = string(value.CustomID)
		base["disabled"] = value.Disabled
		var metadata struct {
			Style int `json:"style"`
		}
		if raw, err := json.Marshal(value); err == nil && json.Unmarshal(raw, &metadata) == nil {
			base["style"] = metadata.Style
		}
	case *discord.StringSelectComponent:
		base["custom_id"] = string(value.CustomID)
		base["placeholder"] = value.Placeholder
		base["disabled"] = value.Disabled
		options := make([]any, 0, len(value.Options))
		for _, option := range value.Options {
			options = append(options, map[string]any{
				"label": option.Label, "value": option.Value,
				"description": option.Description, "default": option.Default,
			})
		}
		base["options"] = options
	case *discord.TextDisplayComponent:
		base["content"] = value.Content
	case *discord.ActionRowComponent:
		children := make([]any, 0, len(*value))
		for _, child := range *value {
			if converted := pluginComponent(child); converted != nil {
				children = append(children, converted)
			}
		}
		base["children"] = children
	case *discord.ContainerComponent:
		base["children"] = pluginChildComponents(value.Components)
	case *discord.SectionComponent:
		base["children"] = pluginChildComponents(value.Components)
		if value.Accessory != nil {
			base["accessory"] = pluginComponent(value.Accessory)
		}
	default:
		return nil
	}
	return base
}

func pluginChildComponents(components []discord.Component) []any {
	out := make([]any, 0, len(components))
	for _, component := range components {
		if value := pluginComponent(component); value != nil {
			out = append(out, value)
		}
	}
	return out
}
