package ui

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
	uitext "awesomeProject/internal/tui/text"
)

// renderMedia renders a message's attachments and stickers as text chips. This
// is the terminal-independent fallback: a real image pass (Kitty graphics) draws
// into the reserved region later, but the chip keeps scrollback readable when
// images are disabled or unavailable.
func (w *ChatView) renderMedia(m store.Message, width int, base screen.Style) []chatLine {
	var lines []chatLine
	muted := mergeStyle(base, w.styles.Cell("messages.attachment"))
	for i, a := range m.Attachments {
		if videoURL, ok := attachmentVideoURL(a); ok {
			// Prefer Discord's JPEG proxy as the poster, but keep the direct URL as
			// the playback target. Without a proxy, reserve a playable placeholder.
			posterURL, _ := attachmentMediaURL(a)
			lines = append(lines, w.mediaLinesVideo(posterURL, videoURL, attachmentChip(a), messageMediaPlacementKey(m, "video", i, videoURL), base, attachmentVideoSpec(a, width, w.mediaMaxRows()), false)...)
			continue
		}
		if url, ok := attachmentMediaURL(a); ok {
			lines = append(lines, w.mediaLines(url, attachmentChip(a), messageMediaPlacementKey(m, "attachment", i, url), base, attachmentMediaSpec(a, url, width, w.mediaMaxRows()), attachmentAnimated(a, url))...)
			continue
		}
		lines = append(lines, chatLine{segments: []chatSegment{{text: attachmentChip(a), style: muted}}})
	}
	for i, s := range m.Stickers {
		if url, ok := stickerMediaURL(s); ok {
			lines = append(lines, w.mediaLines(url, stickerChip(s), messageMediaPlacementKey(m, "sticker", i, url), base, stickerMediaSpec(width), s.Format == store.StickerGIF)...)
			continue
		}
		lines = append(lines, chatLine{segments: []chatSegment{{text: stickerChip(s), style: muted}}})
	}
	return lines
}

func attachmentMediaURL(a store.Attachment) (string, bool) {
	url := a.ProxyURL
	if url == "" {
		url = a.URL
	}
	if url == "" {
		return "", false
	}
	if strings.HasPrefix(a.ContentType, "video/") || media.ClassifyURL(url) == media.ClassVideo {
		// Discord's attachment proxy is the only source guaranteed to negotiate
		// a JPEG poster. Never feed a direct MP4 URL to the image decoder.
		if a.ProxyURL == "" {
			return "", false
		}
		return videoPosterURL(url), true
	}
	if strings.HasPrefix(a.ContentType, "image/") {
		return url, true
	}
	switch media.ClassifyURL(url) {
	case media.ClassImage, media.ClassGIF, media.ClassSticker, media.ClassEmoji:
		return url, true
	default:
		return "", false
	}
}

// attachmentVideoURL returns the direct URL to play for a video attachment.
// The unproxied URL is used because mpv fetches the media itself.
func attachmentVideoURL(a store.Attachment) (string, bool) {
	if a.URL == "" {
		return "", false
	}
	if strings.HasPrefix(a.ContentType, "video/") || media.ClassifyURL(a.URL) == media.ClassVideo {
		return a.URL, true
	}
	return "", false
}

// attachmentVideoSpec sizes a video attachment's play region from its declared
// dimensions, falling back to 16:9 when they are absent.
func attachmentVideoSpec(a store.Attachment, width, maxRows int) mediaSpec {
	sourceW, sourceH := a.W, a.H
	if sourceW <= 0 || sourceH <= 0 {
		sourceW, sourceH = 16, 9
	}
	return mediaSpec{
		maxCols: max(width, 1),
		maxRows: max(maxRows, 1),
		sourceW: sourceW,
		sourceH: sourceH,
	}
}

// attachmentAnimated reports whether an attachment should play as an animated
// GIF rather than a still. Discord tags GIF attachments as image/gif; the URL
// classifier is the fallback when the content type is absent.
func attachmentAnimated(a store.Attachment, url string) bool {
	return a.ContentType == "image/gif" || media.ClassifyURL(url) == media.ClassGIF
}

// videoPosterURL asks Discord's media proxy for a JPEG poster frame. The
// source attachment remains the canonical URL for opening the video; this URL
// is only ever passed through the ordinary image fetcher.
func videoPosterURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("format", "jpeg")
	u.RawQuery = q.Encode()
	return u.String()
}

func stickerMediaURL(s store.Sticker) (string, bool) {
	if s.ID == 0 || s.Format == store.StickerLottie {
		return "", false
	}
	ext := "png"
	if s.Format == store.StickerGIF {
		ext = "gif"
	}
	return fmt.Sprintf("https://media.discordapp.net/stickers/%d.%s?size=160", s.ID, ext), true
}

func attachmentMediaSpec(a store.Attachment, mediaURL string, width, maxRows int) mediaSpec {
	sourceW, sourceH := a.W, a.H
	if queryW, queryH, ok := mediaQuerySize(mediaURL); ok {
		sourceW, sourceH = queryW, queryH
	}
	return mediaSpec{
		maxCols: max(width, 1),
		maxRows: max(maxRows, 1),
		sourceW: sourceW,
		sourceH: sourceH,
	}
}

func stickerMediaSpec(width int) mediaSpec {
	rows := 8
	cols := min(max(width, 1), rows*2)
	rows = max(min(rows, max(cols/2, 1)), 1)
	return mediaSpec{
		maxCols: cols,
		maxRows: rows,
		sourceW: 160,
		sourceH: 160,
		square:  true,
	}
}

func messageMediaPlacementKey(m store.Message, kind string, index int, mediaURL string) string {
	return fmt.Sprintf("%s:%s:%d:%s", messagePlacementPrefix(m), kind, index, mediaURL)
}

func messagePlacementPrefix(m store.Message) string {
	var buf [64]byte
	out := strconv.AppendUint(buf[:0], uint64(m.ChannelID), 10)
	out = append(out, ':')
	if m.ID != 0 {
		out = strconv.AppendUint(out, uint64(m.ID), 10)
	} else {
		out = append(out, "pending:"...)
		out = append(out, m.Nonce...)
	}
	return string(out)
}

// attachmentChip returns the one-line label for an attachment. Videos get a play
// glyph and duration-free size; images and files get a framed filename.
func attachmentChip(a store.Attachment) string {
	label := a.Filename
	if label == "" {
		label = "attachment"
	}
	size := humanSize(a.Size)
	if strings.HasPrefix(a.ContentType, "video/") || media.ClassifyURL(a.URL) == media.ClassVideo {
		return fmt.Sprintf("▶ %s (%s)", label, size)
	}
	return fmt.Sprintf("▒▒ %s (%s) ▒▒", label, size)
}

// stickerChip returns the label for a sticker. Lottie stickers cannot be
// rendered in a terminal, so they always degrade to a named chip.
func stickerChip(s store.Sticker) string {
	return fmt.Sprintf("[sticker: %s]", s.Name)
}

// renderReactions renders the reactions summary line, embedding custom emoji
// images when media rendering is enabled.
// A reaction the current user added is drawn in reverse video. It returns ok=false
// when there are no reactions.
func (w *ChatView) renderReactions(reactions []store.Reaction, placementPrefix string) (chatLine, bool) {
	if len(reactions) == 0 {
		return chatLine{}, false
	}
	base := mergeStyle(w.styles.Cell("messages.content"), w.styles.Cell("messages.reaction"))
	segs := []chatSegment{{text: "⤷ ", style: base}}
	used := uitext.Width("⤷ ")
	var inline []positionedInlineMedia
	spinner := false
	for i, r := range reactions {
		if i > 0 {
			segs = append(segs, chatSegment{text: " · ", style: base})
			used += uitext.Width(" · ")
		}
		style := base
		if r.Me {
			style = mergeStyle(style, w.styles.Cell("messages.reaction.selected"))
		}
		emojiURL := reactionEmojiURL(r)
		if emojiURL != "" && w.mediaCfg.Enabled && w.mediaCfg.EmojiImages {
			const emojiCols = 2
			state := w.ensureMedia(emojiURL, false)
			if state != nil && state.loading {
				placeholder := mediaSpinner(w.spinner) + " "
				segs = append(segs, chatSegment{text: placeholder, style: style})
				used += uitext.Width(placeholder)
				spinner = true
				count := fmt.Sprintf(" %d", r.Count)
				segs = append(segs, chatSegment{text: count, style: style})
				used += uitext.Width(count)
				continue
			}
			if state != nil && state.err == nil && state.img != nil {
				variant := w.mediaVariant(state, mediaSpec{
					maxCols: emojiCols,
					maxRows: 1,
					sourceW: 48,
					sourceH: 48,
					square:  true,
				})
				segs = append(segs, chatSegment{text: "  ", style: style})
				inline = append(inline, positionedInlineMedia{
					col: used,
					media: &inlineMedia{
						url:          emojiURL,
						label:        r.EmojiName,
						placementKey: fmt.Sprintf("%s:reaction:%d:%s", placementPrefix, i, emojiURL),
						cols:         emojiCols,
						rows:         1,
						img:          variant.img,
						style:        style,
					},
				})
				used += emojiCols
				count := fmt.Sprintf(" %d", r.Count)
				segs = append(segs, chatSegment{text: count, style: style})
				used += uitext.Width(count)
				continue
			}
			// A failed or temporarily unqueued fetch must not leave a permanent
			// blank where the reaction should be. Fall through to Unicode/name text.
		}
		label := reactionLabel(r)
		segs = append(segs, chatSegment{text: label, style: style})
		used += uitext.Width(label)
	}
	return chatLine{segments: segs, inlineMedia: inline, spinner: spinner}, true
}

// reactionEmojiURL returns an image URL for both Discord custom emoji and
// standard Unicode emoji. Unicode assets use Twemoji so reactions remain
// visible even when the terminal font has no color-emoji glyphs.
func reactionEmojiURL(r store.Reaction) string {
	if r.EmojiID != 0 {
		return customEmojiURLParts(r.EmojiID, r.EmojiName, r.Animated)
	}
	var codepoints []string
	for _, cp := range r.EmojiName {
		// Twemoji asset names omit the emoji-presentation selector. Joiners,
		// skin tones, keycap combiners, and regional indicators stay intact.
		if cp == '\ufe0f' {
			continue
		}
		codepoints = append(codepoints, fmt.Sprintf("%x", cp))
	}
	if len(codepoints) == 0 {
		return ""
	}
	return "https://cdn.jsdelivr.net/gh/jdecked/twemoji@16.0.1/assets/72x72/" +
		strings.Join(codepoints, "-") + ".png"
}

// reactionLabel formats one reaction as "<emoji> <count>". Custom emoji use
// their name without Discord's colon-delimited source syntax.
func reactionLabel(r store.Reaction) string {
	return fmt.Sprintf("%s %d", r.EmojiName, r.Count)
}

// humanSize formats a byte count as a compact human-readable string.
func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
