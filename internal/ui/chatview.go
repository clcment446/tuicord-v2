package ui

import (
	"strings"

	"awesomeProject/internal/markup"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

// ChatView renders the active channel's messages, bottom-aligned so the newest
// message sits just above the composer. It reads the store live during Draw, so
// no explicit refresh is needed when new messages arrive — a redraw suffices.
type ChatView struct {
	store    *store.Store
	active   func() store.ChannelID
	resolver func() markup.Resolver
	styles   Styles
	scroll   int
	node     layout.Node
}

// Styles is the resolved palette the UI draws with.
type Styles struct {
	Text    screen.Style
	Muted   screen.Style
	Accent  screen.Style
	Border  screen.Style
	Pending screen.Style
	Error   screen.Style
}

// NewChatView returns a chat view over st. active reports which channel to show;
// resolver (optional) resolves mentions and channel references for markup.
func NewChatView(st *store.Store, active func() store.ChannelID, resolver func() markup.Resolver, styles Styles) *ChatView {
	return &ChatView{
		store:    st,
		active:   active,
		resolver: resolver,
		styles:   styles,
		node:     layout.Node{Grow: 1},
	}
}

// displayContent resolves Discord markup in content into a flat display string
// (mentions/channels/emoji resolved, markdown delimiters stripped).
func (w *ChatView) displayContent(content string) string {
	var res markup.Resolver
	if w.resolver != nil {
		res = w.resolver()
	}
	var b strings.Builder
	for _, span := range markup.Parse(content, res) {
		b.WriteString(span.Text)
	}
	return b.String()
}

// Measure fills available space.
func (w *ChatView) Measure(avail tui.Size) tui.Size { return avail }

// Layout returns the layout node.
func (w *ChatView) Layout() *layout.Node { return &w.node }

// CanFocus lets the chat view take focus for scrolling.
func (w *ChatView) CanFocus() bool { return true }

// Draw renders wrapped message lines, newest at the bottom.
func (w *ChatView) Draw(r screen.Region) {
	fill(r, w.styles.Text)
	if r.Width() <= 0 || r.Height() <= 0 {
		return
	}
	lines := w.render(r.Width())
	// Bottom-align: show the last r.Height() lines, offset by scroll.
	start := max(len(lines)-r.Height()-w.scroll, 0)
	end := min(start+r.Height(), len(lines))
	y := 0
	for i := start; i < end; i++ {
		drawChatLine(r, 0, y, lines[i])
		y++
	}
}

type chatLine struct {
	text     string
	style    screen.Style
	segments []chatSegment
}

type chatSegment struct {
	text  string
	style screen.Style
}

// render turns the active channel's messages into wrapped, styled lines. Each
// message contributes a role-colored author line, its wrapped text content, and
// then any rich blocks: media chips, embeds, and a reactions line.
func (w *ChatView) render(width int) []chatLine {
	channel := w.active()
	guild := w.guildOf(channel)
	msgs := w.store.Messages(channel)
	var lines []chatLine
	for _, m := range msgs {
		style := w.styles.Text
		switch {
		case m.Failed:
			style = w.styles.Error
		case m.Pending:
			style = w.styles.Pending
		}
		header := m.Author
		if m.Failed {
			header += " (failed)"
		} else if m.Pending {
			header += " (sending…)"
		}
		// Author line (role-colored when the member has a colored role), then
		// wrapped content indented under it.
		authorStyle := w.styles.Accent
		if color := w.store.MemberColor(guild, m.AuthorID); color != 0 {
			authorStyle.Fg = rgbColor(color)
		}
		lines = append(lines, chatLine{text: header, style: authorStyle})
		if m.Content != "" && !suppressContent(m) {
			lines = append(lines, w.renderContent(m.Content, width, style)...)
		}
		lines = append(lines, w.renderMedia(m, style)...)
		lines = append(lines, w.renderEmbeds(m, width, style)...)
		if line, ok := w.renderReactions(m.Reactions); ok {
			lines = append(lines, line)
		}
	}
	return lines
}

// guildOf reports the guild that owns a channel, or 0 when unknown.
func (w *ChatView) guildOf(channel store.ChannelID) store.GuildID {
	if c, ok := w.store.Channel(channel); ok {
		return c.GuildID
	}
	return 0
}

func (w *ChatView) renderContent(content string, width int, base screen.Style) []chatLine {
	var res markup.Resolver
	if w.resolver != nil {
		res = w.resolver()
	}
	var lines []chatLine
	var line []chatSegment
	used := 0
	flush := func() {
		lines = append(lines, chatLine{segments: line})
		line = nil
		used = 0
	}
	for _, span := range markup.Parse(content, res) {
		style := w.markupStyle(span.Kind, base)
		if span.FG != 0 {
			style.Fg = rgbColor(span.FG)
		}
		parts := strings.Split(span.Text, "\n")
		for i, part := range parts {
			if i > 0 {
				flush()
			}
			for cluster := range text.Clusters(part) {
				if cluster.Width == 0 {
					continue
				}
				if width > 0 && used > 0 && used+cluster.Width > width {
					flush()
				}
				line = appendChatSegment(line, chatSegment{text: cluster.Text, style: style})
				used += cluster.Width
			}
		}
	}
	if len(line) > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

func (w *ChatView) markupStyle(kind markup.Kind, base screen.Style) screen.Style {
	style := base
	switch kind {
	case markup.Kind_Bold:
		style.Attrs |= screen.Bold
	case markup.Kind_Italic:
		style.Attrs |= screen.Italic
	case markup.Kind_Code, markup.Kind_CodeBlock:
		style = mergeStyle(style, w.styles.Muted)
	case markup.Kind_Underline:
		style.Attrs |= screen.Underline
	case markup.Kind_Strike:
		style.Attrs |= screen.Strike
	case markup.Kind_Spoiler:
		// No hover in a TUI, so mask the text as a reverse-video block.
		style.Attrs |= screen.Reverse
	case markup.Kind_Link:
		style.Attrs |= screen.Underline
	case markup.Kind_Quote:
		style = mergeStyle(style, w.styles.Muted)
	case markup.Kind_Header:
		style = mergeStyle(style, w.styles.Accent)
		style.Attrs |= screen.Bold | screen.Underline
	case markup.Kind_Mention, markup.Kind_ChannelMention, markup.Kind_RoleMention:
		style = mergeStyle(style, w.styles.Accent)
	case markup.Kind_MessageLink, markup.Kind_ChannelLink, markup.Kind_InviteLink:
		style = mergeStyle(style, w.styles.Accent)
		style.Attrs |= screen.Underline
	case markup.Kind_Timestamp:
		style = mergeStyle(style, w.styles.Muted)
	}
	return style
}

// rgbColor converts a 0xRRGGBB value into a screen color.
func rgbColor(c uint32) screen.Color {
	return screen.RGB(uint8(c>>16), uint8(c>>8), uint8(c))
}

func appendChatSegment(segments []chatSegment, next chatSegment) []chatSegment {
	if next.text == "" {
		return segments
	}
	if len(segments) > 0 && segments[len(segments)-1].style == next.style {
		segments[len(segments)-1].text += next.text
		return segments
	}
	return append(segments, next)
}

func drawChatLine(r screen.Region, x, y int, line chatLine) {
	if len(line.segments) == 0 {
		drawText(r, x, y, line.text, line.style)
		return
	}
	for _, segment := range line.segments {
		x = drawText(r, x, y, segment.text, segment.style)
		if x >= r.Width() {
			return
		}
	}
}

func mergeStyle(base, overlay screen.Style) screen.Style {
	if overlay.Fg.Set() {
		base.Fg = overlay.Fg
	}
	if overlay.Bg.Set() {
		base.Bg = overlay.Bg
	}
	base.Attrs |= overlay.Attrs
	return base
}

// Handle scrolls the chat view.
func (w *ChatView) Handle(ev tui.Event) bool {
	switch ev := ev.(type) {
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		switch ev.Key {
		case input.KeyUp:
			w.scrollUp()
			return true
		case input.KeyDown:
			w.scrollDown()
			return true
		}
	case input.MouseEvent:
		if ev.Kind != input.MouseWheel {
			return false
		}
		switch ev.Btn {
		case input.ButtonWheelUp:
			w.scrollUp()
			return true
		case input.ButtonWheelDown:
			w.scrollDown()
			return true
		}
	}
	return false
}

func (w *ChatView) scrollUp() {
	w.scroll++
}

func (w *ChatView) scrollDown() {
	if w.scroll > 0 {
		w.scroll--
	}
}
