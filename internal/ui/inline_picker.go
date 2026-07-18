package ui

import (
	"context"
	"image"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"awesomeProject/internal/media"
	"awesomeProject/internal/picker"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	tuitext "awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// InlinePicker is the autocomplete menu activated from the composer by :, %,
// #, @, or &. It deliberately owns its query while open: the composer retains
// the literal trigger text until a result is chosen or the menu is dismissed.
type InlinePicker struct {
	styles   Styles
	trigger  rune
	query    string
	entries  []searchEntry
	filtered []pickerEntry

	list   *widget.ItemList
	header *widget.Text
	hint   *widget.Text
	body   *widget.Node
	node   layout.Node

	onInsert         func(string)
	onSticker        func(uint64)
	onClose          func()
	onQueryChange    func(string)
	onTriggerDelete  func()
	onSwitch         func(store.GuildID, store.ChannelID)
	mediaFetcher     *media.Fetcher
	mediaCfg         media.Config
	mediaCtx         context.Context
	mediaCancel      context.CancelFunc
	mediaJobs        chan string
	post             func(func())
	media            map[string]*pickerMediaState
	active           store.GuildID
	activeChannel    store.ChannelID
	favoriteEmojis   map[string]bool
	favoriteStickers map[uint64]bool
}

// NewInlinePicker creates a composer autocomplete menu. active is the current
// guild, which controls native emoji and sticker availability.
func NewInlinePicker(st *store.Store, styles Styles, active store.GuildID, activeChannel store.ChannelID, nitro, fakeNitro bool, trigger rune, query string, onInsert func(string), onSticker func(uint64), onClose func()) *InlinePicker {
	p := &InlinePicker{
		styles: styles, trigger: trigger, query: query, active: active, activeChannel: activeChannel, onInsert: onInsert,
		onSticker: onSticker, onClose: onClose, list: widget.NewItemList(nil),
		header: widget.NewText(""), hint: widget.NewText(""), node: layout.Node{Grow: 1},
	}
	p.list.SetStyle(styles.Text)
	p.list.SetSelectedStyle(styles.Accent)
	p.header.SetStyle(styles.Text)
	p.header.SetWrap(false)
	p.hint.SetStyle(styles.Muted)
	p.hint.SetWrap(false)
	p.hint.SetContent("↑/↓ move · enter insert · esc close")
	title := "Autocomplete"
	if trigger == '+' {
		title = "Jump to channel"
		p.hint.SetContent(`fuzzy search · \server · \server#channel · ↑/↓ move · enter jump`)
	}
	p.entries = inlineEntries(st, active, activeChannel, nitro, fakeNitro, trigger)
	p.body = widget.Column(titled(title, p.header), titled("Matches", p.list), p.hint)
	p.body.Children()[0].Layout().Basis = 3
	p.body.Children()[0].Layout().Grow = 0
	p.body.Children()[1].Layout().Grow = 1
	p.body.Children()[2].Layout().Basis = 1
	p.body.Children()[2].Layout().Grow = 0
	p.refilter()
	return p
}

func inlineEntries(st *store.Store, active store.GuildID, activeChannel store.ChannelID, nitro, fakeNitro bool, trigger rune) []searchEntry {
	var entries []searchEntry
	switch trigger {
	case ':':
		for _, e := range picker.Unicode() {
			entries = append(entries, searchEntry{key: strings.ToLower(e.Name + " " + strings.Join(e.Keywords, " ")), entry: pickerEntry{label: e.Char + "  :" + e.Name + ":", insert: e.Char, usable: true, favoriteKey: "u:" + e.Char}})
		}
		entries = append(entries, buildCustomEntries(st, active, nitro, fakeNitro)...)
	case '%':
		entries = buildStickerEntries(st, active, nitro, fakeNitro)
	case '#':
		for _, c := range st.Channels(active) {
			if c.Kind == store.ChannelCategory {
				continue
			}
			entries = append(entries, searchEntry{key: strings.ToLower(c.Name), entry: pickerEntry{label: "#" + c.Name, insert: "<#" + idString(uint64(c.ID)) + ">", usable: true}})
		}
	case '@':
		members := st.Members(active)
		if channel, ok := st.Channel(activeChannel); ok && channel.Kind == store.ChannelDM {
			members = append([]store.Member(nil), channel.Recipients...)
		}
		sort.Slice(members, func(i, j int) bool { return strings.ToLower(members[i].Name) < strings.ToLower(members[j].Name) })
		for _, m := range members {
			entries = append(entries, searchEntry{key: strings.ToLower(m.Name), entry: pickerEntry{label: "@" + m.Name, insert: "<@" + idString(uint64(m.ID)) + ">", usable: true}})
		}
	case '&':
		for _, r := range st.Roles(active) {
			entries = append(entries, searchEntry{key: strings.ToLower(r.Name), entry: pickerEntry{label: "&" + r.Name, insert: "<@&" + idString(uint64(r.ID)) + ">", usable: true}})
		}
	case '+':
		for _, g := range st.Guilds() {
			for _, c := range st.Channels(g.ID) {
				if c.Kind == store.ChannelCategory || c.Kind == store.ChannelVoice {
					continue
				}
				label := g.Name + " › #" + c.Name
				if c.Kind == store.ChannelDM {
					label = "@" + c.Name
				}
				entries = append(entries, searchEntry{
					key: strings.ToLower(label), guildKey: strings.ToLower(g.Name), channelKey: strings.ToLower(c.Name),
					entry: pickerEntry{label: label, usable: true, switchGuild: g.ID, switchChannel: c.ID},
				})
			}
		}
	}
	return entries
}

func idString(id uint64) string { return strconv.FormatUint(id, 10) }

func (p *InlinePicker) refilter() {
	selected := p.list.Selected()
	type scored struct {
		entry pickerEntry
		score int
	}
	q := strings.ToLower(strings.TrimSpace(p.query))
	matches := make([]scored, 0, len(p.entries))
	for _, candidate := range p.entries {
		if score, ok := p.matchEntry(candidate, q); ok {
			matches = append(matches, scored{candidate.entry, score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		leftTier, rightTier := p.entryTier(matches[i].entry), p.entryTier(matches[j].entry)
		if leftTier != rightTier {
			return leftTier < rightTier
		}
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].entry.label < matches[j].entry.label
	})
	p.filtered = p.filtered[:0]
	if len(matches) > maxPickerResults {
		matches = matches[:maxPickerResults]
	}
	items := make([]widget.Item, 0, len(matches))
	thumbnailLoads := 0
	for _, match := range matches {
		p.filtered = append(p.filtered, match.entry)
		item := widget.Item{Label: match.entry.label}
		if match.entry.mediaURL != "" && thumbnailLoads < maxPickerThumbnailLoad {
			thumbnailLoads++
			img := p.emojiImage(match.entry.mediaURL)
			if img != nil {
				b := img.Bounds()
				item.Label = "  " + item.Label
				item.Graphic = &widget.ItemGraphic{Image: img, ImageID: stableImageID(match.entry.mediaURL), PlacementID: stableImageID("inline-picker:" + match.entry.favoriteKey), PixelWidth: b.Dx(), PixelHeight: b.Dy(), Cols: 2}
			}
		}
		if !match.entry.usable {
			item.Style = p.styles.Muted
		}
		items = append(items, item)
	}
	p.list.SetItems(items)
	p.list.SetSelectedSilent(selected)
	p.header.SetContent(string(p.trigger) + p.query + "▏")
}

// matchEntry keeps ordinary fuzzy search unchanged. A leading backslash opts
// into server-aware matching: \server matches the server name, while
// \server#channel requires independent fuzzy matches for both parts.
func (p *InlinePicker) matchEntry(candidate searchEntry, query string) (int, bool) {
	if p.trigger != '+' || !strings.HasPrefix(query, `\`) {
		return fuzzyScore(candidate.key, query)
	}
	structured := strings.TrimSpace(strings.TrimPrefix(query, `\`))
	serverQuery, channelQuery, hasChannel := strings.Cut(structured, "#")
	serverScore, ok := fuzzyScore(candidate.guildKey, strings.TrimSpace(serverQuery))
	if !ok {
		return 0, false
	}
	if !hasChannel {
		return serverScore, true
	}
	channelScore, ok := fuzzyScore(candidate.channelKey, strings.TrimSpace(channelQuery))
	if !ok {
		return 0, false
	}
	return serverScore + channelScore, true
}

// SetFavorites supplies the persisted favorite catalogs used to prioritize
// custom and unicode emoji, plus stickers, in composer autocomplete.
func (p *InlinePicker) SetFavorites(emojis []string, stickers []uint64) {
	p.favoriteEmojis = make(map[string]bool, len(emojis))
	for _, key := range emojis {
		p.favoriteEmojis[key] = true
	}
	p.favoriteStickers = make(map[uint64]bool, len(stickers))
	for _, id := range stickers {
		p.favoriteStickers[id] = true
	}
	p.refilter()
}

func (p *InlinePicker) entryTier(entry pickerEntry) int {
	if p.favoriteEmojis[entry.favoriteKey] || p.favoriteStickers[entry.favoriteStickerID] {
		return 0
	}
	if entry.guildID != 0 && entry.guildID == p.active {
		return 1
	}
	return 2
}

// fuzzyScore rewards compact, early subsequence matches. An empty query lists
// every candidate in its stable catalog order.
func fuzzyScore(candidate, query string) (int, bool) {
	// '*' is a lightweight `.*`: it separates ordered fuzzy fragments.
	query = strings.ReplaceAll(query, "*", "")
	if query == "" {
		return 0, true
	}
	pos, score := 0, 0
	for _, want := range query {
		found := false
		for pos < len(candidate) {
			r, size := utf8.DecodeRuneInString(candidate[pos:])
			if r == want {
				score += 10
				if pos == 0 || candidate[pos-1] == ' ' || candidate[pos-1] == '_' || candidate[pos-1] == '-' {
					score += 4
				}
				pos += size
				found = true
				break
			}
			pos += size
		}
		if !found {
			return 0, false
		}
	}
	return score - pos, true
}

// SetQueryChange receives the current query after user edits it.
func (p *InlinePicker) SetQueryChange(fn func(string))                    { p.onQueryChange = fn }
func (p *InlinePicker) SetTriggerDelete(fn func())                        { p.onTriggerDelete = fn }
func (p *InlinePicker) SetSwitch(fn func(store.GuildID, store.ChannelID)) { p.onSwitch = fn }
func (p *InlinePicker) SetMedia(fetcher *media.Fetcher, cfg media.Config, post func(func())) {
	if p.mediaCancel != nil {
		p.mediaCancel()
	}
	p.mediaCtx, p.mediaCancel = context.WithCancel(context.Background())
	cfg = cfg.Bounded()
	p.mediaFetcher, p.mediaCfg, p.post = fetcher, cfg, post
	p.mediaJobs = make(chan string, cfg.QueuedFetches)
	if fetcher != nil && post != nil && cfg.Enabled && cfg.EmojiImages {
		for range cfg.ConcurrentFetches {
			go p.mediaWorker(p.mediaCtx, p.mediaJobs)
		}
	}
	if p.media == nil {
		p.media = map[string]*pickerMediaState{}
	}
	p.refilter()
}
func (p *InlinePicker) Close() {
	if p != nil && p.mediaCancel != nil {
		p.mediaCancel()
		p.mediaCancel = nil
	}
}
func (p *InlinePicker) emojiImage(url string) image.Image {
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
func (p *InlinePicker) mediaWorker(ctx context.Context, jobs <-chan string) {
	for {
		select {
		case <-ctx.Done():
			return
		case url := <-jobs:
			img, _ := p.mediaFetcher.Fetch(ctx, url)
			if ctx.Err() == nil {
				p.post(func() { p.media[url] = &pickerMediaState{img: img}; p.refilter() })
			}
		}
	}
}
func (p *InlinePicker) Children() []tui.Widget          { return []tui.Widget{p.body} }
func (p *InlinePicker) Measure(avail tui.Size) tui.Size { return p.body.Measure(avail) }
func (p *InlinePicker) Layout() *layout.Node {
	p.node.Children = []*layout.Node{p.body.Layout()}
	return &p.node
}
func (p *InlinePicker) Draw(screen.Region)   {}
func (p *InlinePicker) CanFocus() bool       { return true }
func (p *InlinePicker) PreferredFocus() bool { return true }

func (p *InlinePicker) Handle(ev tui.Event) bool {
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
	case input.KeyUp, input.KeyDown, input.KeyHome, input.KeyEnd, input.KeyPageUp, input.KeyPageDown:
		return p.list.Handle(ev)
	case input.KeyBackspace:
		if p.query == "" {
			if p.onTriggerDelete != nil {
				p.onTriggerDelete()
			}
			p.onClose()
			return true
		}
		p.query = p.query[:tuitext.PrevBoundary(p.query, len(p.query))]
		p.queryChanged()
		return true
	case input.KeyRune:
		if key.Mods&(input.Ctrl|input.Alt|input.Super) != 0 {
			return false
		}
		p.query += string(key.Rune)
		p.queryChanged()
		return true
	}
	return false
}

func (p *InlinePicker) queryChanged() {
	p.refilter()
	if p.onQueryChange != nil {
		p.onQueryChange(p.query)
	}
}
func (p *InlinePicker) pick() {
	i := p.list.Selected()
	if i < 0 || i >= len(p.filtered) {
		return
	}
	e := p.filtered[i]
	if !e.usable {
		return
	}
	if e.stickerID != 0 {
		if p.onSticker == nil {
			return
		}
		p.onSticker(e.stickerID)
	} else if e.switchChannel != 0 && p.onSwitch != nil {
		p.onSwitch(e.switchGuild, e.switchChannel)
	} else if p.onInsert != nil {
		p.onInsert(e.insert)
	}
	p.onClose()
}
