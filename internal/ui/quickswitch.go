package ui

import (
	"strings"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// switchEntry is one selectable channel in the quick-switcher.
type switchEntry struct {
	guild   store.GuildID
	channel store.ChannelID
	label   string // "guild › #channel"
}

// QuickSwitcher is a filtered channel picker (Ctrl+K). Typing narrows the list;
// Enter opens the selection; Esc closes it.
type QuickSwitcher struct {
	input    *widget.TextInput
	list     *widget.ItemList
	entries  []switchEntry
	filtered []switchEntry
	onPick   func(store.GuildID, store.ChannelID)
	onClose  func()
	body     *widget.Node
	node     layout.Node
}

// NewQuickSwitcher builds a switcher over every text channel in the store.
func NewQuickSwitcher(st *store.Store, styles Styles, onPick func(store.GuildID, store.ChannelID), onClose func()) *QuickSwitcher {
	qs := &QuickSwitcher{
		input:   widget.NewTextInput("Jump to channel…"),
		list:    widget.NewItemList(nil),
		onPick:  onPick,
		onClose: onClose,
		node:    layout.Node{Grow: 1},
	}
	qs.input.SetStyle(styles.Text)
	qs.list.SetSelectedStyle(styles.Accent)
	qs.entries = collectEntries(st)
	qs.applyFilter("")

	// The switcher input holds focus, so filtering and confirmation are driven
	// from its callbacks; navigation and dismissal arrive via Handle (root
	// fallback) because the input leaves those keys unhandled.
	qs.input.OnChange(qs.applyFilter)
	qs.input.OnSubmit(func(string) { qs.pick() })

	qs.body = widget.Column(titled("Quick Switcher", qs.input), titled("Channels", qs.list))
	qs.body.Children()[0].Layout().Basis = 3
	qs.body.Children()[0].Layout().Grow = 0
	qs.body.Children()[1].Layout().Grow = 1
	return qs
}

func collectEntries(st *store.Store) []switchEntry {
	var entries []switchEntry
	for _, g := range st.Guilds() {
		for _, c := range st.Channels(g.ID) {
			if c.Kind != store.ChannelText {
				continue
			}
			entries = append(entries, switchEntry{
				guild:   g.ID,
				channel: c.ID,
				label:   g.Name + " › #" + c.Name,
			})
		}
	}
	return entries
}

func (qs *QuickSwitcher) applyFilter(query string) {
	query = strings.ToLower(strings.TrimSpace(query))
	qs.filtered = qs.filtered[:0]
	items := make([]widget.Item, 0, len(qs.entries))
	for _, e := range qs.entries {
		if query == "" || strings.Contains(strings.ToLower(e.label), query) {
			qs.filtered = append(qs.filtered, e)
			items = append(items, widget.Item{Label: e.label})
		}
	}
	qs.list.SetItems(items)
	qs.list.SetSelected(0)
}

// Children exposes the composed body for retained-tree traversal.
func (qs *QuickSwitcher) Children() []tui.Widget { return []tui.Widget{qs.body} }

// Measure delegates to the body.
func (qs *QuickSwitcher) Measure(avail tui.Size) tui.Size { return qs.body.Measure(avail) }

// Layout returns the switcher layout node.
func (qs *QuickSwitcher) Layout() *layout.Node {
	qs.node.Children = []*layout.Node{qs.body.Layout()}
	return &qs.node
}

// Draw is a no-op; children draw themselves.
func (qs *QuickSwitcher) Draw(screen.Region) {}

// CanFocus lets the switcher receive keys.
func (qs *QuickSwitcher) CanFocus() bool { return true }

// Handle drives filtering, navigation, selection, and dismissal.
func (qs *QuickSwitcher) Handle(ev tui.Event) bool {
	key, ok := ev.(input.KeyEvent)
	if !ok || key.Release {
		return false
	}
	switch key.Key {
	case input.KeyEsc:
		qs.onClose()
		return true
	case input.KeyEnter:
		qs.pick()
		return true
	case input.KeyUp, input.KeyDown:
		return qs.list.Handle(ev)
	}
	return false
}

func (qs *QuickSwitcher) pick() {
	i := qs.list.Selected()
	if i < 0 || i >= len(qs.filtered) {
		return
	}
	e := qs.filtered[i]
	qs.onPick(e.guild, e.channel)
	qs.onClose()
}
