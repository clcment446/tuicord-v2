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
func (w *ChatView) renderMedia(m store.Message, base screen.Style) []chatLine {
	var lines []chatLine
	muted := mergeStyle(base, w.styles.Muted)
	for _, a := range m.Attachments {
		lines = append(lines, chatLine{segments: []chatSegment{{text: attachmentChip(a), style: muted}}})
	}
	for _, s := range m.Stickers {
		lines = append(lines, chatLine{segments: []chatSegment{{text: stickerChip(s), style: muted}}})
	}
	return lines
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
