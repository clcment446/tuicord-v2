package ui

import (
	"strings"

	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
)

// renderEmbeds renders a message's embeds as chat blocks. Each embed is drawn
// with a colored gutter bar down its left edge, matching Discord's layout.
func (w *ChatView) renderEmbeds(m store.Message, width int, base screen.Style) []chatLine {
	var lines []chatLine
	for _, e := range m.Embeds {
		lines = append(lines, w.renderEmbed(e, width, base)...)
	}
	return lines
}

func (w *ChatView) renderEmbed(e store.Embed, width int, base screen.Style) []chatLine {
	gutterStyle := base
	if e.Color != 0 {
		gutterStyle.Fg = rgbColor(e.Color)
	}
	gutter := chatSegment{text: "▍ ", style: gutterStyle}
	var lines []chatLine
	add := func(text string, style screen.Style) {
		lines = append(lines, chatLine{segments: []chatSegment{gutter, {text: text, style: style}}})
	}

	if e.AuthorName != "" {
		add(e.AuthorName, mergeStyle(base, w.styles.Muted))
	}
	if e.Title != "" {
		title := base
		title.Attrs |= screen.Bold
		if e.URL != "" {
			title.Attrs |= screen.Underline
		}
		add(e.Title, title)
	}
	// Description flows through the markup renderer, then gets the gutter prefixed.
	inner := width - 2
	for _, line := range w.renderContent(e.Description, inner, base) {
		line.segments = append([]chatSegment{gutter}, line.segments...)
		lines = append(lines, line)
	}
	for _, f := range e.Fields {
		name := base
		name.Attrs |= screen.Bold
		add(f.Name, name)
		add(f.Value, base)
	}
	if chip := embedMediaChip(e); chip != "" {
		add(chip, mergeStyle(base, w.styles.Muted))
	}
	if e.FooterText != "" {
		add(e.FooterText, mergeStyle(base, w.styles.Muted))
	}
	return lines
}

// embedMediaChip returns a one-line chip describing an embed's media, or "" when
// the embed carries none.
func embedMediaChip(e store.Embed) string {
	switch {
	case e.VideoURL != "":
		return "▶ video"
	case e.Kind == store.EmbedGIFV:
		return "[GIF]"
	case e.ImageURL != "":
		return "▒▒ image ▒▒"
	case e.ThumbURL != "":
		return "▒▒ thumbnail ▒▒"
	default:
		return ""
	}
}

// suppressContent reports whether a message's raw text should be hidden because
// it is exactly one media URL that an embed or classifier already renders — this
// is Discord parity for gif links and fake-nitro sticker/emoji links.
func suppressContent(m store.Message) bool {
	content := strings.TrimSpace(m.Content)
	if content == "" || strings.ContainsAny(content, " \t\n") {
		return false
	}
	if !strings.HasPrefix(content, "http") {
		return false
	}
	for _, e := range m.Embeds {
		if e.Kind == store.EmbedGIFV || e.Kind == store.EmbedImage || e.Kind == store.EmbedVideo {
			return true
		}
	}
	switch media.ClassifyURL(content) {
	case media.ClassGIF, media.ClassImage, media.ClassVideo, media.ClassSticker, media.ClassEmoji:
		return true
	default:
		return false
	}
}
