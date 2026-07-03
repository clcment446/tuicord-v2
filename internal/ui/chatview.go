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
		drawText(r, 0, y, lines[i].text, lines[i].style)
		y++
	}
}

type chatLine struct {
	text  string
	style screen.Style
}

// render turns the active channel's messages into wrapped, styled lines.
func (w *ChatView) render(width int) []chatLine {
	msgs := w.store.Messages(w.active())
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
		// Author line, then wrapped content indented under it.
		lines = append(lines, chatLine{text: header, style: w.styles.Accent})
		for _, wrapped := range text.Wrap(w.displayContent(m.Content), width) {
			lines = append(lines, chatLine{text: wrapped, style: style})
		}
	}
	return lines
}

// Handle scrolls the chat view.
func (w *ChatView) Handle(ev tui.Event) bool {
	key, ok := ev.(input.KeyEvent)
	if !ok || key.Release {
		return false
	}
	switch key.Key {
	case input.KeyUp:
		w.scroll++
		return true
	case input.KeyDown:
		if w.scroll > 0 {
			w.scroll--
		}
		return true
	}
	return false
}
