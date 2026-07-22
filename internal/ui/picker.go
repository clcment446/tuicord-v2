package ui

import (
	"context"
	"image"
	"sort"
	"strconv"
	"strings"
	"sync"

	appdiscord "awesomeProject/internal/discord"
	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
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
	tabGIF
	tabSticker
)

var pickerTabNames = [...]string{"Emoji", "Custom", "GIF", "Sticker"}

// Keep opening a picker cheap even when the account can see very large emoji
// catalogs. Search narrows beyond this limit; thumbnail requests are capped
// separately because they are remote work.
const (
	maxPickerResults       = 100
	maxPickerThumbnailLoad = 12
)

// GIFResult is the picker-facing projection of a Discord GIF search result.
type GIFResult = appdiscord.GIFResult

// GIFSearchFunc performs a GIF search off the UI thread and delivers its
// result back on the UI thread.
type GIFSearchFunc func(string, func([]GIFResult, error))

// pickerEntry is one selectable result. insert is the string dropped into the
// composer; usable is false when the account cannot send it (no Nitro and
// fake-nitro disabled), in which case picking it is a no-op.
type pickerEntry struct {
	label             string
	insert            string
	usable            bool
	stickerID         uint64
	recentStickerID   uint64
	favoriteKey       string
	favoriteStickerID uint64
	mediaURL          string
	guildID           store.GuildID
	switchGuild       store.GuildID
	switchChannel     store.ChannelID
}

// searchEntry pairs a precomputed lowercase search key with its entry.
type searchEntry struct {
	key        string
	guildKey   string
	channelKey string
	entry      pickerEntry
}

// Picker is the emoji/sticker overlay opened over the composer (Ctrl+E). It owns
// its own query string and tab state and drives a single results list, so all
// keys route to it: ←/→ switch tabs, ↑/↓ move, Enter inserts, Esc closes, and
// typing filters. Custom emoji and stickers the account cannot use natively fall
// back to marked CDN links when fake-nitro is enabled (see internal/picker).
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

	filtered         []pickerEntry
	onInsert         func(string)
	onClose          func()
	onSticker        func(uint64)
	onStickerRecent  func(uint64)
	onFavorite       func(string, uint64)
	favoriteEmojis   map[string]bool
	favoriteStickers map[uint64]bool
	mediaFetcher     *media.Fetcher
	mediaCfg         media.Config
	mediaCtx         context.Context
	mediaCancel      context.CancelFunc
	mediaJobs        chan string
	mediaWG          sync.WaitGroup
	post             func(func())
	media            map[string]*pickerMediaState
	searchGIF        GIFSearchFunc
	gifQuery         string
	gifResults       []pickerEntry

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
	p.list.SetStyle(styles.Cell("picker"))
	p.list.SetSelectedStyle(styles.Cell("picker.selected"))
	p.queryText.SetStyle(styles.Cell("picker.query"))
	p.queryText.SetWrap(false)
	p.tabText.SetStyle(styles.Cell("picker"))
	p.tabText.SetWrap(false)
	p.hintText.SetStyle(styles.Cell("picker.hint"))
	p.hintText.SetWrap(false)
	p.hintText.SetContent("←/→ tabs · ↑/↓ move · alt-f fav · enter insert · esc close")

	p.custom = buildCustomEntries(st, active, nitro, fakeNitro)
	p.stickers = buildStickerEntries(st, active, nitro, fakeNitro)

	p.body = widget.Column(
		titled(styles, "Picker — type to search", p.queryText),
		p.tabText,
		titled(styles, "Results", p.list),
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
// entries, marking whether each is native, a fake-nitro link, or locked.
func buildCustomEntries(st *store.Store, active store.GuildID, nitro, fakeNitro bool) []searchEntry {
	var out []searchEntry
	for _, g := range st.Guilds() {
		sameGuild := g.ID == active
		for _, e := range st.GuildEmojis(g.ID) {
			text, ok := picker.EmojiInsert(e.ID, e.Name, e.Animated, sameGuild, nitro, fakeNitro)
			label := ":" + e.Name + ":"
			if ok && strings.HasPrefix(text, "["+markup.FakeEmojiMarker) {
				label += "  (fake-nitro)"
			} else if !ok {
				label += "  (locked)"
			}
			out = append(out, searchEntry{
				key:   strings.ToLower(e.Name),
				entry: pickerEntry{label: label, insert: text, usable: ok, favoriteKey: "e:" + strconv.FormatUint(e.ID, 10), mediaURL: customEmojiURLParts(e.ID, e.Name, e.Animated), guildID: g.ID},
			})
		}
	}
	return out
}

// buildStickerEntries resolves every guild's stickers into insertable entries.
func buildStickerEntries(st *store.Store, active store.GuildID, nitro, fakeNitro bool) []searchEntry {
	var out []searchEntry
	for _, g := range st.Guilds() {
		for _, s := range st.GuildStickers(g.ID) {
			native := g.ID == active || nitro
			text, ok := "", native
			stickerID := s.ID
			if !native {
				text, ok = picker.StickerInsert(s.ID, s.Name, fakeNitro)
				stickerID = 0
			}
			label := s.Name
			if native {
				label += "  (native)"
			} else if ok {
				label += "  (fake-nitro)"
			}
			if !ok {
				label += "  (locked)"
			}
			out = append(out, searchEntry{
				key:   strings.ToLower(s.Name),
				entry: pickerEntry{label: label, insert: text, usable: ok, stickerID: stickerID, recentStickerID: s.ID, favoriteStickerID: s.ID, guildID: g.ID},
			})
		}
	}
	return out
}

// SetStickerSelect installs the native-sticker send callback.
func (p *Picker) SetStickerSelect(selectSticker func(uint64)) { p.onSticker = selectSticker }

// SetStickerRecent installs the callback used to persist a successful sticker selection.
func (p *Picker) SetStickerRecent(record func(uint64)) { p.onStickerRecent = record }

// SetMedia enables cached asynchronous custom-emoji thumbnails in picker rows.
func (p *Picker) SetMedia(fetcher *media.Fetcher, cfg media.Config, post func(func())) {
	if p.mediaCancel != nil {
		p.mediaCancel()
		p.mediaWG.Wait()
	}
	p.mediaCtx, p.mediaCancel = context.WithCancel(context.Background())
	cfg = cfg.Bounded()
	p.mediaFetcher, p.mediaCfg, p.post = fetcher, cfg, post
	p.mediaJobs = make(chan string, cfg.QueuedFetches)
	if fetcher != nil && post != nil && cfg.Enabled && cfg.EmojiImages {
		for range cfg.ConcurrentFetches {
			p.mediaWG.Add(1)
			go p.mediaWorker(p.mediaCtx, p.mediaJobs)
		}
	}
	if p.media == nil {
		p.media = map[string]*pickerMediaState{}
	}
	p.refilter()
}

// Close cancels thumbnail requests when the picker overlay is dismissed.
func (p *Picker) Close() {
	if p != nil && p.mediaCancel != nil {
		p.mediaCancel()
		p.mediaCancel = nil
		p.mediaWG.Wait()
	}
}

// SetFavorites supplies the persisted favorite catalogs and the persistence callback.
func (p *Picker) SetFavorites(emojis []string, stickers []uint64, toggle func(string, uint64)) {
	p.favoriteEmojis = make(map[string]bool, len(emojis))
	for _, key := range emojis {
		p.favoriteEmojis[key] = true
	}
	p.favoriteStickers = make(map[uint64]bool, len(stickers))
	for _, id := range stickers {
		p.favoriteStickers[id] = true
	}
	p.onFavorite = toggle
	p.refilter()
}

// SetRecentStickers moves the supplied sticker IDs to the head of the sticker
// catalog, preserving recent order and then catalog order for the remainder.
func (p *Picker) SetRecentStickers(ids []uint64) {
	byID := make(map[uint64]searchEntry, len(p.stickers))
	for _, entry := range p.stickers {
		if entry.entry.recentStickerID != 0 {
			byID[entry.entry.recentStickerID] = entry
		}
	}
	ordered := make([]searchEntry, 0, len(p.stickers))
	seen := make(map[uint64]bool, len(ids))
	for _, id := range ids {
		if entry, ok := byID[id]; ok && !seen[id] {
			entry.entry.label = "Recent · " + entry.entry.label
			ordered = append(ordered, entry)
			seen[id] = true
		}
	}
	for _, entry := range p.stickers {
		if !seen[entry.entry.recentStickerID] {
			ordered = append(ordered, entry)
		}
	}
	p.stickers = ordered
	p.refilter()
}

func (p *Picker) refilter() {
	selected := p.list.Selected()
	q := strings.ToLower(strings.TrimSpace(p.query))
	p.filtered = p.filtered[:0]
	switch p.tab {
	case tabEmoji:
		for _, e := range picker.FilterEmoji(p.query, 300) {
			p.filtered = append(p.filtered, pickerEntry{
				label:  e.Char + "  :" + e.Name + ":",
				insert: e.Char,
				usable: true, favoriteKey: "u:" + e.Char,
			})
		}
	case tabCustom:
		p.appendMatches(p.custom, q)
	case tabGIF:
		p.filtered = append(p.filtered, p.gifResults...)
		p.searchGIFs(strings.TrimSpace(p.query))
	case tabSticker:
		p.appendMatches(p.stickers, q)
	}

	p.prioritizeFavorites()
	if len(p.filtered) > maxPickerResults {
		p.filtered = p.filtered[:maxPickerResults]
	}
	items := make([]widget.Item, len(p.filtered))
	thumbnailLoads := 0
	for i, e := range p.filtered {
		label := e.label
		if p.favorite(e) {
			label = "★ " + label
		}
		item := widget.Item{Label: label}
		if e.mediaURL != "" && thumbnailLoads < maxPickerThumbnailLoad {
			thumbnailLoads++
			img := p.emojiImage(e.mediaURL)
			if img != nil {
				b := img.Bounds()
				item.Label = "  " + item.Label
				item.Graphic = &widget.ItemGraphic{Image: img, ImageID: stableImageID(e.mediaURL), PlacementID: stableImageID("picker:" + e.favoriteKey), PixelWidth: b.Dx(), PixelHeight: b.Dy(), Cols: 2, Z: -1}
			}
		}
		if !e.usable {
			item.Style = p.styles.Cell("muted")
		}
		items[i] = item
	}
	p.list.SetItems(items)
	p.list.SetSelectedSilent(selected)
	p.updateHeader()
}

type pickerMediaState struct {
	img     image.Image
	loading bool
}

func (p *Picker) emojiImage(url string) image.Image {
	if url == "" || !p.mediaCfg.Enabled || !p.mediaCfg.EmojiImages || p.mediaFetcher == nil || p.post == nil {
		return nil
	}
	if state := p.media[url]; state != nil {
		return state.img
	}
	p.media[url] = &pickerMediaState{loading: true}
	select {
	case p.mediaJobs <- url:
	default:
		delete(p.media, url)
	}
	return nil
}

func (p *Picker) mediaWorker(ctx context.Context, jobs <-chan string) {
	defer p.mediaWG.Done()
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case url := <-jobs:
			if ctx.Err() != nil {
				return
			}
			img, _ := p.mediaFetcher.Fetch(ctx, url)
			if ctx.Err() == nil {
				p.post(func() {
					if ctx.Err() == nil {
						p.media[url] = &pickerMediaState{img: img}
						p.refilter()
					}
				})
			}
		}
	}
}

func (p *Picker) favorite(e pickerEntry) bool {
	return p.favoriteEmojis[e.favoriteKey] || p.favoriteStickers[e.favoriteStickerID]
}
func (p *Picker) prioritizeFavorites() {
	sort.SliceStable(p.filtered, func(i, j int) bool { return p.favorite(p.filtered[i]) && !p.favorite(p.filtered[j]) })
}
func (p *Picker) toggleFavorite() {
	i := p.list.Selected()
	if i < 0 || i >= len(p.filtered) {
		return
	}
	e := p.filtered[i]
	if p.favoriteEmojis == nil {
		p.favoriteEmojis = map[string]bool{}
	}
	if p.favoriteStickers == nil {
		p.favoriteStickers = map[uint64]bool{}
	}
	if e.favoriteStickerID != 0 {
		p.favoriteStickers[e.favoriteStickerID] = !p.favoriteStickers[e.favoriteStickerID]
	} else if e.favoriteKey != "" {
		p.favoriteEmojis[e.favoriteKey] = !p.favoriteEmojis[e.favoriteKey]
	} else {
		return
	}
	if p.onFavorite != nil {
		p.onFavorite(e.favoriteKey, e.favoriteStickerID)
	}
	p.refilter()
}

// SetGIFSearch installs the asynchronous Discord GIF search callback.
func (p *Picker) SetGIFSearch(search GIFSearchFunc) {
	p.searchGIF = search
	if p.tab == tabGIF {
		p.searchGIFs(strings.TrimSpace(p.query))
	}
}

func (p *Picker) searchGIFs(query string) {
	if p.searchGIF == nil || query == "" || query == p.gifQuery {
		return
	}
	p.gifQuery = query
	p.searchGIF(query, func(results []GIFResult, err error) {
		if err != nil || p.tab != tabGIF || strings.TrimSpace(p.query) != query {
			return
		}
		entries := make([]pickerEntry, 0, len(results))
		for _, result := range results {
			resultURL := result.URL
			if resultURL == "" {
				resultURL = result.Src
			}
			if resultURL == "" {
				continue
			}
			label := result.Title
			if label == "" {
				label = "GIF"
			}
			entries = append(entries, pickerEntry{label: label, insert: resultURL, usable: true})
		}
		p.gifResults = entries
		p.refilter()
	})
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
		if key.Rune == 'f' && key.Mods&input.Alt != 0 {
			p.toggleFavorite()
			return true
		}
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
		if e.stickerID == 0 || p.onSticker == nil {
			return
		}
		if p.onStickerRecent != nil {
			p.onStickerRecent(e.recentStickerID)
		}
		p.onSticker(e.stickerID)
		p.onClose()
		return
	}
	if e.recentStickerID != 0 && p.onStickerRecent != nil {
		p.onStickerRecent(e.recentStickerID)
	}
	p.onInsert(e.insert)
	p.onClose()
}
