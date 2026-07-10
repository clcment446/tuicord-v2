package ui

import (
	"strings"

	"awesomeProject/internal/picker"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// pickerTab identifies the picker's three content tabs.
type pickerTab int

const (
	tabEmoji pickerTab = iota
	tabCustom
	tabSticker
)

var pickerTabNames = [...]string{"Emoji", "Custom", "Sticker"}

// pickerEntry is one selectable result. insert is the string dropped into the
// composer; usable is false when the account cannot send it (no Nitro and
// fake-nitro disabled), in which case picking it is a no-op.
type pickerEntry struct {
	label  string
	insert string
	usable bool
}

// searchEntry pairs a precomputed lowercase search key with its entry.
type searchEntry struct {
	key   string
	entry pickerEntry
}

// Picker is the emoji/sticker overlay opened over the composer (Ctrl+E). It owns
// its own query string and tab state and drives a single results list, so all
// keys route to it: ←/→ switch tabs, ↑/↓ move, Enter inserts, Esc closes, and
// typing filters. Custom emoji and stickers the account cannot use natively fall
// back to their CDN URL when fake-nitro is enabled (see internal/picker).
type Picker struct {
	styles Styles

	query string
	tab   pickerTab

	list      *widget.ItemList
	queryText *widget.Text
	tabText   *widget.Text
	hintText  *widget.Text

	custom   []searchEntry
	stickers []searchEntry

	filtered []pickerEntry
	onInsert func(string)
	onClose  func()

	body *widget.Node
	node layout.Node
}

// NewPicker builds a picker over the store's emoji/sticker catalogs. active is
// the guild whose custom emoji count as "same guild" for native use; nitro and
// fakeNitro come from READY and config respectively.
func NewPicker(st *store.Store, styles Styles, active store.GuildID, nitro, fakeNitro bool, onInsert func(string), onClose func()) *Picker {
	p := &Picker{
		styles:    styles,
		list:      widget.NewItemList(nil),
		queryText: widget.NewText(""),
		tabText:   widget.NewText(""),
		hintText:  widget.NewText(""),
		onInsert:  onInsert,
		onClose:   onClose,
		node:      layout.Node{Grow: 1},
	}
	p.list.SetStyle(styles.Text)
	p.list.SetSelectedStyle(styles.Accent)
	p.queryText.SetStyle(styles.Text)
	p.queryText.SetWrap(false)
	p.tabText.SetWrap(false)
	p.hintText.SetStyle(styles.Muted)
	p.hintText.SetWrap(false)
	p.hintText.SetContent("←/→ tabs · ↑/↓ move · enter insert · esc close")

	p.custom = buildCustomEntries(st, active, nitro, fakeNitro)
	p.stickers = buildStickerEntries(st, fakeNitro)

	p.body = widget.Column(
		titled("Picker — type to search", p.queryText),
		p.tabText,
		titled("Results", p.list),
		p.hintText,
	)
	p.body.Children()[0].Layout().Basis = 3
	p.body.Children()[0].Layout().Grow = 0
	p.body.Children()[1].Layout().Basis = 1
	p.body.Children()[1].Layout().Grow = 0
	p.body.Children()[2].Layout().Grow = 1
	p.body.Children()[3].Layout().Basis = 1
	p.body.Children()[3].Layout().Grow = 0

	p.refilter()
	return p
}

// buildCustomEntries resolves every guild's custom emoji into insertable
// entries, marking whether each is native, a fake-nitro URL, or locked.
func buildCustomEntries(st *store.Store, active store.GuildID, nitro, fakeNitro bool) []searchEntry {
	var out []searchEntry
	for _, g := range st.Guilds() {
		sameGuild := g.ID == active
		for _, e := range st.GuildEmojis(g.ID) {
			text, ok := picker.EmojiInsert(e.ID, e.Name, e.Animated, sameGuild, nitro, fakeNitro)
			label := ":" + e.Name + ":"
			if ok && strings.HasPrefix(text, "http") {
				label += "  (fake-nitro)"
			} else if !ok {
				label += "  (locked)"
			}
			out = append(out, searchEntry{
				key:   strings.ToLower(e.Name),
				entry: pickerEntry{label: label, insert: text, usable: ok},
			})
		}
	}
	return out
}

// buildStickerEntries resolves every guild's stickers into insertable entries.
func buildStickerEntries(st *store.Store, fakeNitro bool) []searchEntry {
	var out []searchEntry
	for _, g := range st.Guilds() {
		for _, s := range st.GuildStickers(g.ID) {
			text, ok := picker.StickerInsert(s.ID, fakeNitro)
			label := s.Name
			if !ok {
				label += "  (locked)"
			}
			out = append(out, searchEntry{
				key:   strings.ToLower(s.Name),
				entry: pickerEntry{label: label, insert: text, usable: ok},
			})
		}
	}
	return out
}

func (p *Picker) refilter() {
	q := strings.ToLower(strings.TrimSpace(p.query))
	p.filtered = p.filtered[:0]
	switch p.tab {
	case tabEmoji:
		for _, e := range picker.FilterEmoji(p.query, 300) {
			p.filtered = append(p.filtered, pickerEntry{
				label:  e.Char + "  :" + e.Name + ":",
				insert: e.Char,
				usable: true,
			})
		}
	case tabCustom:
		p.appendMatches(p.custom, q)
	case tabSticker:
		p.appendMatches(p.stickers, q)
	}

	items := make([]widget.Item, len(p.filtered))
	for i, e := range p.filtered {
		item := widget.Item{Label: e.label}
		if !e.usable {
			item.Style = p.styles.Muted
		}
		items[i] = item
	}
	p.list.SetItems(items)
	p.list.SetSelectedSilent(0)
	p.updateHeader()
}

func (p *Picker) appendMatches(entries []searchEntry, q string) {
	for _, e := range entries {
		if q == "" || strings.Contains(e.key, q) {
			p.filtered = append(p.filtered, e.entry)
		}
	}
}

func (p *Picker) updateHeader() {
	cursor := "▏"
	p.queryText.SetContent(p.query + cursor)
	var b strings.Builder
	for i, name := range pickerTabNames {
		if i > 0 {
			b.WriteString("  ")
		}
		if pickerTab(i) == p.tab {
			b.WriteString("[" + name + "]")
		} else {
			b.WriteString(" " + name + " ")
		}
	}
	p.tabText.SetContent(b.String())
}

func (p *Picker) setTab(t pickerTab) {
	n := pickerTab(len(pickerTabNames))
	// Wrap around within the tab range.
	p.tab = (t%n + n) % n
	p.refilter()
}

// Children exposes the composed body for retained-tree traversal.
func (p *Picker) Children() []tui.Widget { return []tui.Widget{p.body} }

// Measure delegates to the body.
func (p *Picker) Measure(avail tui.Size) tui.Size { return p.body.Measure(avail) }

// Layout returns the picker layout node.
func (p *Picker) Layout() *layout.Node {
	p.node.Children = []*layout.Node{p.body.Layout()}
	return &p.node
}

// Draw is a no-op; children draw themselves.
func (p *Picker) Draw(screen.Region) {}

// CanFocus lets the picker own keyboard focus.
func (p *Picker) CanFocus() bool { return true }

// PreferredFocus makes the picker the initial focus so all keys route to it.
func (p *Picker) PreferredFocus() bool { return true }

// Handle drives tab switching, list navigation, query editing, and selection.
func (p *Picker) Handle(ev tui.Event) bool {
	key, ok := ev.(input.KeyEvent)
	if !ok || key.Release {
		return false
	}
	switch key.Key {
	case input.KeyEsc:
		p.onClose()
		return true
	case input.KeyEnter:
		p.pick()
		return true
	case input.KeyLeft:
		p.setTab(p.tab - 1)
		return true
	case input.KeyRight:
		p.setTab(p.tab + 1)
		return true
	case input.KeyUp, input.KeyDown, input.KeyHome, input.KeyEnd,
		input.KeyPageUp, input.KeyPageDown:
		return p.list.Handle(ev)
	case input.KeyBackspace:
		p.backspace()
		return true
	case input.KeyRune:
		if key.Mods&(input.Ctrl|input.Alt|input.Super) != 0 {
			return false
		}
		p.query += string(key.Rune)
		p.refilter()
		return true
	}
	return false
}

func (p *Picker) backspace() {
	if p.query == "" {
		return
	}
	r := []rune(p.query)
	p.query = string(r[:len(r)-1])
	p.refilter()
}

func (p *Picker) pick() {
	i := p.list.Selected()
	if i < 0 || i >= len(p.filtered) {
		return
	}
	e := p.filtered[i]
	if !e.usable || e.insert == "" {
		return
	}
	p.onInsert(e.insert)
	p.onClose()
}
