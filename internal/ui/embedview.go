package ui

import (
	"strings"

	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
	uitext "awesomeProject/internal/tui/text"
)

// renderEmbeds renders a message's embeds as chat blocks.
func (w *ChatView) renderEmbeds(m store.Message, width int, base screen.Style) []chatLine {
	var lines []chatLine
	for i, e := range m.Embeds {
		lines = append(lines, w.renderEmbed(m, e, i, width, base)...)
	}
	return lines
}

func (w *ChatView) renderEmbed(m store.Message, e store.Embed, index int, width int, base screen.Style) []chatLine {
	if pureMediaEmbed(e) {
		chip := embedMediaChip(e)
		if url, ok := embedMediaURL(e); ok {
			return w.mediaLines(url, chip, messageMediaPlacementKey(m, "embed", index, url), base, embedMediaSpec(e, url, width, w.mediaMaxRows()))
		}
		if chip != "" {
			return []chatLine{{segments: []chatSegment{{text: chip, style: mergeStyle(base, w.styles.Muted)}}}}
		}
		return nil
	}

	accent := w.embedAccent(e)
	bg := w.embedBackground(accent, base)
	borderStyle := base
	borderStyle.Fg = accent
	borderStyle.Bg = bg
	contentBase := base
	contentBase.Bg = bg

	inner := max(width-2, 1)
	var content []chatLine
	add := func(s string, style screen.Style) {
		style.Bg = bg
		content = append(content, embedPlainLines(s, inner, style)...)
	}

	if e.AuthorName != "" {
		add(e.AuthorName, mergeStyle(contentBase, w.styles.Muted))
	}
	if e.Title != "" {
		title := contentBase
		title.Attrs |= screen.Bold
		if e.URL != "" {
			title.Attrs |= screen.Underline
		}
		add(e.Title, title)
	}
	for _, line := range w.renderContent(e.Description, inner, contentBase) {
		content = append(content, line)
	}
	for _, f := range e.Fields {
		name := contentBase
		name.Attrs |= screen.Bold
		add(f.Name, name)
		add(f.Value, contentBase)
	}
	if chip := embedMediaChip(e); chip != "" {
		add(chip, mergeStyle(contentBase, w.styles.Muted))
	}
	if url, ok := embedMediaURL(e); ok {
		for _, line := range w.mediaLines(url, embedMediaChip(e), messageMediaPlacementKey(m, "embed", index, url), contentBase, embedMediaSpec(e, url, inner, w.mediaMaxRows())) {
			if line.media != nil {
				content = append(content, line)
				continue
			}
			for i := range line.segments {
				line.segments[i].style.Bg = bg
			}
			content = append(content, line)
		}
	}
	if e.FooterText != "" {
		add(e.FooterText, mergeStyle(contentBase, w.styles.Muted))
	}
	if len(content) == 0 {
		content = append(content, embedPlainLines("[embed]", inner, mergeStyle(contentBase, w.styles.Muted))...)
	}
	return frameEmbedLines(content, inner, borderStyle, contentBase)
}

func pureMediaEmbed(e store.Embed) bool {
	if e.AuthorName != "" || e.Title != "" || e.Description != "" || len(e.Fields) > 0 || e.FooterText != "" {
		return false
	}
	return e.Kind == store.EmbedGIFV || e.Kind == store.EmbedImage || e.Kind == store.EmbedVideo
}

func (w *ChatView) embedAccent(e store.Embed) screen.Color {
	if e.Color != 0 {
		return rgbColor(e.Color)
	}
	if w.styles.Accent.Fg.Set() {
		return w.styles.Accent.Fg
	}
	return screen.RGB(88, 101, 242)
}

func (w *ChatView) embedBackground(accent screen.Color, base screen.Style) screen.Color {
	bg := screen.RGB(12, 14, 18)
	if base.Bg.Set() {
		bg = base.Bg
	}
	return mixColor(bg, accent, 18)
}

func mixColor(base, accent screen.Color, accentPercent int) screen.Color {
	basePercent := 100 - accentPercent
	return screen.RGB(
		uint8((int(base.R)*basePercent+int(accent.R)*accentPercent)/100),
		uint8((int(base.G)*basePercent+int(accent.G)*accentPercent)/100),
		uint8((int(base.B)*basePercent+int(accent.B)*accentPercent)/100),
	)
}

func embedPlainLines(s string, width int, style screen.Style) []chatLine {
	if s == "" {
		return nil
	}
	var lines []chatLine
	var segments []chatSegment
	used := 0
	flush := func() {
		lines = append(lines, chatLine{segments: segments})
		segments = nil
		used = 0
	}
	for _, part := range strings.Split(s, "\n") {
		if used > 0 || len(segments) > 0 {
			flush()
		}
		for cluster := range uitext.Clusters(part) {
			if cluster.Width == 0 {
				continue
			}
			if width > 0 && used > 0 && used+cluster.Width > width {
				flush()
			}
			segments = appendChatSegment(segments, chatSegment{text: cluster.Text, style: style})
			used += cluster.Width
		}
	}
	if len(segments) > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

func frameEmbedLines(content []chatLine, innerWidth int, borderStyle, contentStyle screen.Style) []chatLine {
	top := chatLine{segments: []chatSegment{{text: "╭" + strings.Repeat("─", innerWidth) + "╮", style: borderStyle}}}
	bottom := chatLine{segments: []chatSegment{{text: "╰" + strings.Repeat("─", innerWidth) + "╯", style: borderStyle}}}
	lines := []chatLine{top}
	for _, line := range content {
		if line.media != nil {
			lines = append(lines, line)
			continue
		}
		framed := chatLine{
			segments: []chatSegment{{text: "│", style: borderStyle}},
			message:  line.message,
		}
		used := 0
		if line.text != "" && len(line.segments) == 0 {
			line.segments = []chatSegment{{text: line.text, style: line.style}}
		}
		for _, segment := range line.segments {
			segment.style.Bg = contentStyle.Bg
			framed.segments = append(framed.segments, segment)
			used += uitext.Width(segment.text)
		}
		if pad := innerWidth - used; pad > 0 {
			framed.segments = append(framed.segments, chatSegment{text: strings.Repeat(" ", pad), style: contentStyle})
		}
		framed.segments = append(framed.segments, chatSegment{text: "│", style: borderStyle})
		for _, inline := range line.inlineMedia {
			inline.col++
			framed.inlineMedia = append(framed.inlineMedia, inline)
		}
		framed.actions = appendOffsetComponentHits(framed.actions, line.actions, 1)
		lines = append(lines, framed)
	}
	lines = append(lines, bottom)
	return lines
}

func appendOffsetComponentHits(dst, src []componentHit, offset int) []componentHit {
	for _, hit := range src {
		hit.start += offset
		hit.end += offset
		dst = append(dst, hit)
	}
	return dst
}

func embedMediaURL(e store.Embed) (string, bool) {
	switch {
	case e.ImageURL != "":
		return e.ImageURL, true
	case e.ThumbURL != "":
		return e.ThumbURL, true
	default:
		return "", false
	}
}

func embedMediaSpec(e store.Embed, mediaURL string, width, maxRows int) mediaSpec {
	spec := mediaSpec{maxCols: max(width, 1), maxRows: max(maxRows, 1)}
	if queryW, queryH, ok := mediaQuerySize(mediaURL); ok {
		spec.sourceW, spec.sourceH = queryW, queryH
	}
	if e.ThumbURL != "" && mediaURL == e.ThumbURL && e.ImageURL == "" {
		spec.maxCols = min(spec.maxCols, 24)
		spec.maxRows = min(spec.maxRows, 8)
	}
	return spec
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
