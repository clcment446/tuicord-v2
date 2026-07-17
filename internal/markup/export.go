package markup

import (
	"fmt"
	"strings"

	"awesomeProject/internal/store"
)

// ExportMessage returns a copy-friendly Markdown representation of a Discord
// message. Content is deliberately returned verbatim so Discord markup and
// entity syntax survive copying; supplementary Discord structures are appended
// as readable Markdown rather than their gateway JSON representation.
func ExportMessage(message store.Message) string {
	parts := make([]string, 0, 5)
	if message.Content != "" {
		parts = append(parts, message.Content)
	}
	if section := exportAttachments(message.Attachments); section != "" {
		parts = append(parts, section)
	}
	if section := exportStickers(message.Stickers); section != "" {
		parts = append(parts, section)
	}
	if section := exportEmbeds(message.Embeds); section != "" {
		parts = append(parts, section)
	}
	if section := exportComponents(message.ComponentTree, message.Components); section != "" {
		parts = append(parts, section)
	}
	return strings.Join(parts, "\n\n")
}

// ExportMessages formats messages as a single copy-friendly Markdown document.
// A thematic break retains the boundary between selected Discord messages.
func ExportMessages(messages []store.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		if exported := ExportMessage(message); exported != "" {
			parts = append(parts, exported)
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func exportAttachments(attachments []store.Attachment) string {
	if len(attachments) == 0 {
		return ""
	}
	lines := []string{"## Attachments"}
	for _, attachment := range attachments {
		label := attachment.Filename
		if label == "" {
			label = "attachment"
		}
		item := "- " + markdownLink(label, attachment.URL)
		meta := make([]string, 0, 2)
		if attachment.ContentType != "" {
			meta = append(meta, attachment.ContentType)
		}
		if attachment.Size > 0 {
			meta = append(meta, humanSize(attachment.Size))
		}
		if len(meta) > 0 {
			item += " (" + strings.Join(meta, ", ") + ")"
		}
		lines = append(lines, item)
	}
	return strings.Join(lines, "\n")
}

func exportStickers(stickers []store.Sticker) string {
	if len(stickers) == 0 {
		return ""
	}
	lines := []string{"## Stickers"}
	for _, sticker := range stickers {
		name := sticker.Name
		if name == "" {
			name = "sticker"
		}
		lines = append(lines, "- "+name+" ("+stickerFormat(sticker.Format)+")")
	}
	return strings.Join(lines, "\n")
}

func exportEmbeds(embeds []store.Embed) string {
	if len(embeds) == 0 {
		return ""
	}
	lines := []string{"## Embeds"}
	for _, embed := range embeds {
		title := embed.Title
		if title == "" {
			title = "Embed"
		}
		lines = append(lines, "### "+markdownLink(title, embed.URL))
		if embed.AuthorName != "" {
			lines = append(lines, "**Author:** "+embed.AuthorName)
		}
		if embed.Description != "" {
			lines = append(lines, embed.Description)
		}
		for _, field := range embed.Fields {
			name := field.Name
			if name == "" {
				name = "Field"
			}
			lines = append(lines, "- **"+name+":** "+field.Value)
		}
		if embed.FooterText != "" {
			lines = append(lines, "_"+embed.FooterText+"_")
		}
		if embed.Provider != "" {
			lines = append(lines, "**Provider:** "+embed.Provider)
		}
		for _, media := range []struct{ label, url string }{
			{"image", embed.ImageURL}, {"thumbnail", embed.ThumbURL}, {"video", embed.VideoURL},
		} {
			if media.url != "" {
				lines = append(lines, "!["+media.label+"]("+media.url+")")
			}
		}
	}
	return strings.Join(lines, "\n")
}

func exportComponents(tree []store.ComponentNode, legacy []store.Component) string {
	if len(tree) == 0 && len(legacy) == 0 {
		return ""
	}
	lines := []string{"## Components"}
	if len(tree) > 0 {
		for _, node := range tree {
			appendComponentMarkdown(&lines, node)
		}
	} else {
		for _, component := range legacy {
			appendLegacyComponentMarkdown(&lines, component)
		}
	}
	return strings.Join(lines, "\n")
}

func appendComponentMarkdown(lines *[]string, node store.ComponentNode) {
	switch node.Kind {
	case store.ComponentTextDisplay:
		appendLine(lines, node.Content)
	case store.ComponentButton:
		appendLine(lines, "- Button: "+componentLabel(node))
	case store.ComponentLinkButton:
		appendLine(lines, "- Link button: "+markdownLink(componentLabel(node), node.URL))
	case store.ComponentSelect:
		appendLine(lines, "- Select: "+componentLabel(node))
		appendOptions(lines, node.Options)
	case store.ComponentRadioGroup:
		appendLine(lines, "- Radio group: "+componentLabel(node))
		appendOptions(lines, node.Options)
	case store.ComponentCheckboxGroup:
		appendLine(lines, "- Checkbox group: "+componentLabel(node))
		appendOptions(lines, node.Options)
	case store.ComponentCheckbox:
		appendLine(lines, "- Checkbox: "+componentLabel(node))
	case store.ComponentTextInput:
		appendLine(lines, "- Text input: "+componentLabel(node))
	case store.ComponentFileUpload:
		appendLine(lines, "- File upload: "+componentLabel(node))
	case store.ComponentThumbnail:
		appendComponentMedia(lines, "Thumbnail", node.Media)
	case store.ComponentMediaGallery:
		appendComponentMedia(lines, "Media", node.Media)
	case store.ComponentFile:
		appendComponentMedia(lines, "File", node.Media)
	case store.ComponentSeparator:
		if node.Divider {
			appendLine(lines, "---")
		}
	case store.ComponentUnknown:
		appendLine(lines, fmt.Sprintf("- Unknown component (type %d)", node.RawType))
	}
	if node.Accessory != nil {
		appendComponentMarkdown(lines, *node.Accessory)
	}
	for _, child := range node.Children {
		appendComponentMarkdown(lines, child)
	}
}

func appendLegacyComponentMarkdown(lines *[]string, component store.Component) {
	label := component.Label
	if label == "" {
		label = "component"
	}
	switch component.Kind {
	case store.ComponentLinkButton:
		appendLine(lines, "- Link button: "+markdownLink(label, component.URL))
	case store.ComponentButton:
		appendLine(lines, "- Button: "+label)
	case store.ComponentSelect:
		appendLine(lines, "- Select: "+label)
	default:
		appendLine(lines, "- Component: "+label)
	}
}

func appendOptions(lines *[]string, options []store.ComponentOption) {
	for _, option := range options {
		label := option.Label
		if label == "" {
			label = option.Value
		}
		if option.Description != "" {
			label += " — " + option.Description
		}
		appendLine(lines, "  - "+label)
	}
}

func appendComponentMedia(lines *[]string, kind string, media []store.ComponentMedia) {
	if len(media) == 0 {
		appendLine(lines, "- "+kind)
		return
	}
	for _, item := range media {
		label := item.Description
		if label == "" {
			label = item.Name
		}
		if label == "" {
			label = strings.ToLower(kind)
		}
		url := item.URL
		if url == "" {
			url = item.ProxyURL
		}
		appendLine(lines, "- "+kind+": "+markdownLink(label, url))
	}
}

func appendLine(lines *[]string, line string) {
	if line != "" {
		*lines = append(*lines, line)
	}
}

func componentLabel(node store.ComponentNode) string {
	if node.Label != "" {
		return node.Label
	}
	if node.Placeholder != "" {
		return node.Placeholder
	}
	return "component"
}

func markdownLink(label, url string) string {
	if url == "" {
		return label
	}
	return "[" + label + "](" + url + ")"
}

func stickerFormat(format store.StickerFormat) string {
	switch format {
	case store.StickerAPNG:
		return "APNG"
	case store.StickerGIF:
		return "GIF"
	case store.StickerLottie:
		return "Lottie"
	default:
		return "PNG"
	}
}

func humanSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%d KiB", size/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(size)/(1024*1024))
}
