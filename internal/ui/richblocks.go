package ui

import (
	"fmt"
	"strings"

	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
)

// renderMedia renders a message's attachments and stickers as text chips. This
// is the terminal-independent fallback: a real image pass (Kitty graphics) draws
// into the reserved region later, but the chip keeps scrollback readable when
// images are disabled or unavailable.
func (w *ChatView) renderMedia(m store.Message, width int, base screen.Style) []chatLine {
	var lines []chatLine
	muted := mergeStyle(base, w.styles.Muted)
	for i, a := range m.Attachments {
		if url, ok := attachmentMediaURL(a); ok {
			lines = append(lines, w.mediaLines(url, attachmentChip(a), messageMediaPlacementKey(m, "attachment", i, url), base, attachmentMediaSpec(a, url, width, w.mediaMaxRows()))...)
			continue
		}
		lines = append(lines, chatLine{segments: []chatSegment{{text: attachmentChip(a), style: muted}}})
	}
	for i, s := range m.Stickers {
		if url, ok := stickerMediaURL(s); ok {
			lines = append(lines, w.mediaLines(url, stickerChip(s), messageMediaPlacementKey(m, "sticker", i, url), base, stickerMediaSpec(width))...)
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
	id := fmt.Sprintf("pending:%s", m.Nonce)
	if m.ID != 0 {
		id = fmt.Sprintf("%d", m.ID)
	}
	return fmt.Sprintf("%d:%s:%s:%d:%s", m.ChannelID, id, kind, index, mediaURL)
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

// renderReactions renders the reactions summary line, e.g. "⤷ 👍 3 · :pepe: 12".
// A reaction the current user added is drawn in reverse video. It returns ok=false
// when there are no reactions.
func (w *ChatView) renderReactions(reactions []store.Reaction) (chatLine, bool) {
	if len(reactions) == 0 {
		return chatLine{}, false
	}
	base := mergeStyle(w.styles.Text, w.styles.Muted)
	segs := []chatSegment{{text: "⤷ ", style: base}}
	for i, r := range reactions {
		if i > 0 {
			segs = append(segs, chatSegment{text: " · ", style: base})
		}
		style := base
		if r.Me {
			style.Attrs |= screen.Reverse
		}
		segs = append(segs, chatSegment{text: reactionLabel(r), style: style})
	}
	return chatLine{segments: segs}, true
}

// reactionLabel formats one reaction as "<emoji> <count>". Custom emoji show
// their name between colons; unicode emoji show the glyph directly.
func reactionLabel(r store.Reaction) string {
	emoji := r.EmojiName
	if r.EmojiID != 0 {
		emoji = ":" + r.EmojiName + ":"
	}
	return fmt.Sprintf("%s %d", emoji, r.Count)
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
