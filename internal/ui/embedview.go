package ui

import (
	"strconv"
	"strings"

	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
	uitext "awesomeProject/internal/tui/text"
)

// renderEmbeds renders a message's embeds as chat blocks.
func (w *ChatView) renderEmbeds(m store.Message, width int, base screen.Style) []chatLine {
	markedMedia := markedFakeNitroMediaURLs(m.Content)
	var lines []chatLine
	for i, e := range m.Embeds {
		if markedFakeNitroEmbed(e, markedMedia) {
			continue
		}
		embed := w.renderEmbed(m, e, i, width, base)
		if len(embed) > 0 {
			embed[0].embedStart = true
			embed[0].embedKey = messagePlacementPrefix(m) + ":embed:" + strconv.Itoa(i)
		}
		lines = append(lines, embed...)
	}
	return lines
}

// markedFakeNitroMediaURLs returns the CDN URLs that content already renders
// as fake-Nitro media. Matching pure-media embeds must be skipped to avoid
// drawing the same emoji or sticker twice.
func markedFakeNitroMediaURLs(content string) map[string]bool {
	urls := make(map[string]bool)
	for _, span := range markup.Parse(content, markup.Resolver{}) {
		switch span.Kind {
		case markup.Kind_FakeEmoji:
			if media.ClassifyURL(span.URL) != media.ClassEmoji {
				continue
			}
			urls[span.URL] = true
		case markup.Kind_FakeSticker:
			if media.ClassifyURL(span.URL) != media.ClassSticker {
				continue
			}
			urls[span.URL] = true
		}
	}
	return urls
}

// markedFakeNitroEmbed reports whether a minimal Discord media unfurl belongs
// to a fake-Nitro link already rendered from message content. Pretty links use
// EmbedLink and can put the original URL in URL while ImageURL is a proxy.
func markedFakeNitroEmbed(e store.Embed, markedMedia map[string]bool) bool {
	if !unadornedEmbed(e) {
		return false
	}
	for _, url := range []string{e.URL, e.ImageURL, e.ThumbURL, e.VideoURL} {
		if markedMedia[url] {
			return true
		}
	}
	return false
}

func (w *ChatView) renderEmbed(m store.Message, e store.Embed, index int, width int, base screen.Style) []chatLine {
	if pureMediaEmbed(e) {
		chip := embedMediaChip(e)
		posterURL, _ := embedMediaURL(e)
		if posterURL != "" {
			return w.mediaLinesLink(posterURL, embedThumbnailLink(e, posterURL), chip, messageMediaPlacementKey(m, "embed", index, posterURL), base, embedMediaSpec(e, posterURL, width, w.mediaMaxRows()), media.ClassifyURL(posterURL) == media.ClassGIF)
		}
		if chip != "" {
			return []chatLine{{segments: []chatSegment{{text: chip, style: mergeStyle(base, w.styles.Cell("embeds.footer"))}}}}
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
		add(e.AuthorName, mergeStyle(contentBase, w.styles.Cell("embeds.author")))
	}
	if e.Title != "" {
		title := mergeStyle(contentBase, w.styles.Cell("embeds.title"))
		if e.URL != "" {
			title = mergeStyle(title, w.styles.Cell("embeds.title.link"))
		}
		for _, line := range w.renderContent(e.Title, inner, title) {
			content = append(content, line)
		}
	}
	for _, line := range w.renderContent(e.Description, inner, contentBase) {
		content = append(content, line)
	}
	for _, f := range e.Fields {
		name := mergeStyle(contentBase, w.styles.Cell("embeds.field.name"))
		content = append(content, w.renderContent(f.Name, inner, name)...)
		content = append(content, w.renderContent(f.Value, inner, contentBase)...)
	}
	if chip := embedMediaChip(e); chip != "" {
		add(chip, mergeStyle(contentBase, w.styles.Cell("embeds.footer")))
	}
	if posterURL, ok := embedMediaURL(e); ok {
		for _, line := range w.mediaLinesLink(posterURL, embedThumbnailLink(e, posterURL), embedMediaChip(e), messageMediaPlacementKey(m, "embed", index, posterURL), contentBase, embedMediaSpec(e, posterURL, inner, w.mediaMaxRows()), media.ClassifyURL(posterURL) == media.ClassGIF) {
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
		add(e.FooterText, mergeStyle(contentBase, w.styles.Cell("embeds.footer")))
	}
	if len(content) == 0 {
		content = append(content, embedPlainLines("[embed]", inner, mergeStyle(contentBase, w.styles.Cell("embeds.footer")))...)
	}
	return frameEmbedLines(content, inner, borderStyle, contentBase)
}

func embedOpenURL(e store.Embed) string {
	if e.URL != "" {
		return e.URL
	}
	return e.VideoURL
}

func embedThumbnailLink(e store.Embed, posterURL string) string {
	if e.Kind == store.EmbedGIFV || media.ClassifyURL(posterURL) == media.ClassGIF {
		return ""
	}
	return embedOpenURL(e)
}

func pureMediaEmbed(e store.Embed) bool {
	if !unadornedEmbed(e) {
		return false
	}
	return e.Kind == store.EmbedGIFV || e.Kind == store.EmbedImage || e.Kind == store.EmbedVideo
}

func unadornedEmbed(e store.Embed) bool {
	return e.AuthorName == "" && e.Title == "" && e.Description == "" && len(e.Fields) == 0 && e.FooterText == ""
}

func (w *ChatView) embedAccent(e store.Embed) screen.Color {
	if !w.styles.HasCustom("embeds.border") && e.Color != 0 {
		return rgbColor(e.Color)
	}
	if configured := w.styles.Cell("embeds.border"); configured.Fg.Set() {
		return configured.Fg
	}
	return screen.RGB(88, 101, 242)
}

func (w *ChatView) embedBackground(accent screen.Color, base screen.Style) screen.Color {
	bg := w.styles.Cell("embeds.background").Bg
	if !bg.Set() {
		bg = screen.RGB(12, 14, 18)
	}
	if base.Bg.Set() {
		if !w.styles.Cell("embeds.background").Bg.Set() {
			bg = base.Bg
		}
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
	top := chatLine{segments: []chatSegment{{text: "╭" + strings.Repeat("─", innerWidth) + "╮", style: borderStyle}}, restrictHighlight: true}
	bottom := chatLine{segments: []chatSegment{{text: "╰" + strings.Repeat("─", innerWidth) + "╯", style: borderStyle}}, restrictHighlight: true}
	lines := []chatLine{top}
	for _, line := range content {
		framed := translateChatLine(line, 1)
		framed.restrictHighlight = true
		framed.highlightStart = 1
		framed.highlightEnd = innerWidth + 1
		if line.media != nil {
			// Media is drawn as a terminal graphic after the cell layer. Keep
			// the frame in the cell layer so the graphic cannot cover either
			// border, and give the image one cell of horizontal inset.
			framed.segments = []chatSegment{
				{text: "│", style: borderStyle},
				{text: strings.Repeat(" ", innerWidth), style: contentStyle},
				{text: "│", style: borderStyle},
			}
			lines = append(lines, framed)
			continue
		}
		framed.segments = []chatSegment{{text: "│", style: borderStyle}}
		used := 0
		if framed.text != "" && len(framed.segments) == 1 {
			framed.segments = append(framed.segments, chatSegment{text: framed.text, style: framed.style})
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
		lines = append(lines, framed)
	}
	lines = append(lines, bottom)
	return lines
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
	switch media.ClassifyURL(mediaURL) {
	case media.ClassEmoji:
		return mediaSpec{maxCols: 2, maxRows: 1, sourceW: 48, sourceH: 48, square: true}
	case media.ClassSticker:
		return stickerMediaSpec(width)
	}
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
