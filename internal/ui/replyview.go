package ui

import (
	"fmt"
	"strconv"
	"strings"

	"awesomeProject/internal/markup"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
	uitext "awesomeProject/internal/tui/text"
)

// renderReplyLine emits the "╭─▸ @author preview" line above a reply's content,
// showing who was replied to and a one-line excerpt of the original message.
// The author cells carry a user-mention action so activating them opens the
// profile, mirroring inline mentions.
func (w *ChatView) renderReplyLine(reply store.MessageReply, channel store.ChannelID, width int) chatLine {
	guild := w.guildOf(channel)
	muted := w.styles.Cell("muted")
	if reply.Deleted {
		return chatLine{segments: []chatSegment{{text: "╭─▸ original message was deleted", style: muted}}}
	}
	name := reply.Author
	if n, ok := w.store.MemberName(guild, reply.AuthorID); ok && n != "" {
		name = n
	}
	if name == "" {
		name = "unknown"
	}
	authorStyle := w.styles.Cell("messages.author")
	if color := w.store.MemberColor(guild, reply.AuthorID); color != 0 && !w.styles.HasCustom("messages.author") {
		authorStyle.Fg = rgbColor(color)
	}
	prefix := "╭─▸ "
	author := "@" + name
	line := chatLine{segments: []chatSegment{
		{text: prefix, style: muted},
		{text: author, style: authorStyle},
	}}
	if reply.AuthorID != 0 {
		start := uitext.Width(prefix)
		line.entities = []entityHit{{
			start:  start,
			end:    start + uitext.Width(author),
			action: markup.Action{Kind: markup.ActionUserMention, Target: strconv.FormatUint(uint64(reply.AuthorID), 10)},
		}}
	}
	preview := strings.Join(strings.Fields(reply.Content), " ")
	if preview != "" {
		line.segments = append(line.segments, chatSegment{text: "  " + truncateToWidth(preview, width-chatLineWidth(line)-2), style: muted})
	}
	return line
}

// truncateToWidth cuts s at the given cell budget, appending an ellipsis when
// anything was dropped. Grapheme clusters are never split.
func truncateToWidth(s string, budget int) string {
	if budget <= 0 {
		return ""
	}
	if uitext.Width(s) <= budget {
		return s
	}
	var b strings.Builder
	used := 0
	for cluster := range uitext.Clusters(s) {
		if used+cluster.Width > budget-1 {
			break
		}
		b.WriteString(cluster.Text)
		used += cluster.Width
	}
	return b.String() + "…"
}

// renderForwards renders forwarded-message snapshots as a quoted block: a
// "↱ Forwarded" caption followed by the snapshot's content and rich media,
// all behind a left quote bar. Snapshots reuse the normal content, media, and
// embed renderers through a synthetic message keyed off this message's ID so
// media placements stay unique and stable.
func (w *ChatView) renderForwards(m store.Message, width int, base screen.Style) []chatLine {
	if len(m.Forwards) == 0 {
		return nil
	}
	muted := mergeStyle(base, w.styles.Cell("muted"))
	bar := w.styles.Cell("muted")
	inner := max(width-2, 1)
	var lines []chatLine
	for i, f := range m.Forwards {
		caption := "↱ Forwarded"
		if !f.Timestamp.IsZero() {
			caption += " · " + f.Timestamp.Format("2006-01-02 15:04")
		}
		content := []chatLine{{segments: []chatSegment{{text: caption, style: muted}}}}
		if f.Content != "" {
			content = append(content, w.renderContent(f.Content, inner, base)...)
		}
		synthetic := store.Message{
			ChannelID:   m.ChannelID,
			Nonce:       fmt.Sprintf("fwd:%d:%s:%d", m.ID, m.Nonce, i),
			Attachments: f.Attachments,
			Stickers:    f.Stickers,
			Embeds:      f.Embeds,
		}
		content = append(content, w.renderMedia(synthetic, inner, base)...)
		content = append(content, w.renderEmbeds(synthetic, inner, base)...)
		for _, line := range content {
			framed := translateChatLine(line, 2)
			framed.segments = append([]chatSegment{{text: "▍ ", style: bar}}, framed.segments...)
			lines = append(lines, framed)
		}
	}
	return lines
}
