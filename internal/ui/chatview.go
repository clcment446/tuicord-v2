package ui

import (
	"context"
	"hash/fnv"
	"image"
	"net/url"
	"strconv"
	"strings"
	"time"

	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// ChatView renders the active channel's messages, bottom-aligned so the newest
// message sits just above the composer. It reads the store live during Draw, so
// no explicit refresh is needed when new messages arrive — a redraw suffices.
type ChatView struct {
	store      *store.Store
	active     func() store.ChannelID
	resolver   func() markup.Resolver
	onReachTop func()
	styles     Styles
	node       layout.Node

	visibleLines       []chatLine
	contextMessage     store.Message
	contextMessageSet  bool
	componentAction    ComponentAction
	componentActionSet bool
	entityAction       markup.Action
	entityActionSet    bool
	componentFlashes   map[string]time.Time
	expandedComponents map[string]bool
	selectedComponents map[string]map[string]bool
	multiPickers       map[string]bool
	activePicker       componentAction
	activePickerSet    bool

	mediaCfg     media.Config
	mediaFetcher *media.Fetcher
	post         func(func())
	media        map[string]*chatMediaState
	mediaSlots   chan struct{}
	spinner      int
	// mediaLoadingCount tracks in-flight fetches so mediaLoading stays O(1)
	// as w.media grows.
	mediaLoadingCount int
	// spinnerVisible reports whether the last Draw put a spinner on screen.
	// Animating one that is scrolled out of view, or in another channel, would
	// invalidate the frame twice a second for nothing.
	spinnerVisible bool

	// bodyCache memoizes rendered message bodies. Without it every frame
	// re-parses markup and re-lays out embeds and components for the whole
	// channel history. See chatCacheEntry for the invalidation inputs.
	bodyCache map[string]*chatCacheEntry
	// componentEpoch versions the component presentation state this widget
	// owns (expansion, selection, flashes). Anything that changes what a
	// component renders as must bump it.
	componentEpoch uint64
	// renderGen counts renders, so unused cache entries can be swept.
	renderGen uint64
	// bodyDeps collects the media a body touched while collectDeps is set.
	bodyDeps    []mediaDep
	collectDeps bool

	// bottomScroll owns the viewport offset and preserves the reading position
	// when newly rendered lines are appended below it.
	bottomScroll       widget.BottomScroll
	lastMessageChannel store.ChannelID
	lastFirstMessage   store.MessageID
	lastLastMessage    store.MessageID

	// emojiKeyPrefix and emojiSeq assign each inline emoji occurrence of the
	// message currently being rendered a viewport-unique placement key.
	emojiKeyPrefix string
	emojiSeq       int
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
		mediaCfg: media.DefaultConfig(),
		node:     layout.Node{Grow: 1},
	}
}

// OnReachTop registers a callback invoked when the user scrolls toward older
// messages. The callback runs on the UI goroutine.
func (w *ChatView) OnReachTop(fn func()) {
	if w != nil {
		w.onReachTop = fn
	}
}

// SetMedia enables asynchronous inline media loading for attachments, stickers,
// emoji CDN links, and image embeds. post must schedule callbacks on the UI
// goroutine; passing nil leaves text-chip fallbacks in place.
func (w *ChatView) SetMedia(fetcher *media.Fetcher, cfg media.Config, post func(func())) {
	if w == nil {
		return
	}
	w.mediaFetcher = fetcher
	w.mediaCfg = cfg
	w.post = post
	if w.mediaSlots == nil {
		w.mediaSlots = make(chan struct{}, 8)
	}
	if w.media == nil {
		w.media = map[string]*chatMediaState{}
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

func (w *ChatView) mediaLines(url, label, placementKey string, base screen.Style, spec mediaSpec) []chatLine {
	state := w.ensureMedia(url)
	muted := mergeStyle(base, w.styles.Muted)
	switch {
	case state == nil:
		return []chatLine{{segments: []chatSegment{{text: label, style: muted}}}}
	case state.err != nil:
		return []chatLine{{segments: []chatSegment{{text: label + " (failed)", style: muted}}}}
	case state.img == nil:
		return []chatLine{{segments: []chatSegment{{text: label + " " + mediaSpinner(w.spinner), style: muted}}, spinner: true}}
	default:
		variant := w.mediaVariant(state, spec)
		if placementKey == "" {
			placementKey = url
		}
		block := &inlineMedia{url: url, label: label, placementKey: placementKey, cols: variant.cols, rows: variant.rows, img: variant.img, style: base}
		lines := make([]chatLine, variant.rows)
		for i := range lines {
			lines[i] = chatLine{media: block, mediaRow: i}
		}
		return lines
	}
}

func (w *ChatView) ensureMedia(url string) *chatMediaState {
	if w == nil || url == "" || !w.mediaCfg.Enabled {
		return nil
	}
	if w.media == nil {
		w.media = map[string]*chatMediaState{}
	}
	state := w.media[url]
	if state != nil {
		state.touched = w.renderGen
		w.recordMediaDep(url, state)
		return state
	}
	if w.mediaFetcher == nil || w.post == nil {
		return nil
	}
	state = &chatMediaState{loading: true, touched: w.renderGen}
	w.media[url] = state
	w.mediaLoadingCount++
	w.recordMediaDep(url, state)
	go w.fetchMedia(url)
	return state
}

// recordMediaDep notes that the body currently being rendered read state, so
// the cache entry can be invalidated when that media changes.
func (w *ChatView) recordMediaDep(url string, state *chatMediaState) {
	if !w.collectDeps {
		return
	}
	for _, d := range w.bodyDeps {
		if d.url == url {
			return
		}
	}
	w.bodyDeps = append(w.bodyDeps, mediaDep{url: url, rev: state.rev})
}

func mediaSpinner(step int) string {
	frames := [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return "[" + frames[step%len(frames)] + "]"
}

// mediaLoading reports whether any fetch is still in flight. It reads a
// counter maintained by ensureMedia and fetchMedia rather than scanning
// w.media, which grows for the lifetime of the session.
func (w *ChatView) mediaLoading() bool {
	return w.mediaLoadingCount > 0
}

func (w *ChatView) fetchMedia(url string) {
	w.mediaSlots <- struct{}{}
	defer func() { <-w.mediaSlots }()
	img, err := w.mediaFetcher.Fetch(context.Background(), url)
	w.post(func() {
		state := w.media[url]
		if state == nil {
			// The state was dropped while the fetch was in flight; it was not
			// counted as loading, so resurrect it without decrementing.
			state = &chatMediaState{}
			w.media[url] = state
		} else if state.loading {
			w.mediaLoadingCount--
		}
		state.loading = false
		state.img = img
		state.err = err
		state.variants = nil
		// Invalidate any cached body that rendered this media as a placeholder.
		state.rev++
	})
}

func (w *ChatView) mediaMaxRows() int {
	maxRows := w.mediaCfg.MaxHeightCells
	if maxRows <= 0 {
		maxRows = media.DefaultConfig().MaxHeightCells
	}
	return max(maxRows, 1)
}

func (w *ChatView) mediaVariant(state *chatMediaState, spec mediaSpec) chatMediaVariant {
	if state == nil || state.img == nil {
		return chatMediaVariant{cols: 1, rows: 1}
	}
	spec = w.normalizeMediaSpec(spec)
	key := spec.key()
	if state.variants != nil {
		if variant, ok := state.variants[key]; ok {
			return variant
		}
	}
	srcW, srcH := spec.sourceW, spec.sourceH
	if srcW <= 0 || srcH <= 0 {
		b := state.img.Bounds()
		srcW, srcH = b.Dx(), b.Dy()
	}
	if spec.square {
		side := max(srcW, srcH)
		if side <= 0 {
			side = 1
		}
		srcW, srcH = side, side
	}
	cols, rows := fitMediaCells(srcW, srcH, spec.maxCols, spec.maxRows)
	variant := chatMediaVariant{img: state.img, cols: cols, rows: rows}
	if state.variants == nil {
		state.variants = map[string]chatMediaVariant{}
	}
	state.variants[key] = variant
	return variant
}

func (w *ChatView) normalizeMediaSpec(spec mediaSpec) mediaSpec {
	if spec.maxCols <= 0 {
		spec.maxCols = 1
	}
	if spec.maxRows <= 0 {
		spec.maxRows = w.mediaMaxRows()
	}
	spec.maxRows = min(spec.maxRows, w.mediaMaxRows())
	if spec.square {
		spec.maxRows = min(spec.maxRows, max(spec.maxCols/2, 1))
		spec.maxCols = min(spec.maxCols, spec.maxRows*2)
	}
	return spec
}

func (w *ChatView) drawInlineMedia(r screen.Region, x, y int, block *inlineMedia, width int) {
	if block == nil || block.img == nil {
		return
	}
	cols := block.cols
	if cols <= 0 || x+cols > width {
		cols = max(min(width-x, block.cols), 1)
	}
	rows := max(block.rows, 1)
	img := widget.NewKittyImageFrom(block.img).SetID(stableImageID(block.url)).SetStyle(block.style)
	if block.placementKey != "" {
		img.SetPlacementID(stableImageID(block.placementKey))
	}
	if b := block.img.Bounds(); b.Dx() > 0 && b.Dy() > 0 {
		img.SetPixelSize(b.Dx(), b.Dy())
	}
	img.Draw(r.Clip(screen.Rect{X: x, Y: y, W: cols, H: rows}))
}

func stableImageID(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	id := h.Sum32()
	if id == 0 {
		return 1
	}
	return id
}

// Draw renders wrapped message lines, newest at the bottom.
func (w *ChatView) Draw(r screen.Region) {
	fill(r, w.styles.Text)
	if r.Width() <= 0 || r.Height() <= 0 {
		return
	}
	lines := w.render(r.Width())
	channel := w.active()
	messages := w.store.Messages(channel)
	firstMessage := store.MessageID(0)
	lastMessage := store.MessageID(0)
	if len(messages) > 0 {
		firstMessage = messages[0].ID
		lastMessage = messages[len(messages)-1].ID
	}
	prepended := channel == w.lastMessageChannel &&
		w.lastFirstMessage != 0 && firstMessage != 0 &&
		firstMessage != w.lastFirstMessage && lastMessage == w.lastLastMessage
	if prepended {
		w.bottomScroll.UpdatePrepend(len(lines), r.Height())
	} else {
		w.bottomScroll.Update(len(lines), r.Height())
	}
	w.lastMessageChannel = channel
	w.lastFirstMessage = firstMessage
	w.lastLastMessage = lastMessage
	// Bottom-align: show the last r.Height() lines, offset by scroll.
	start := max(len(lines)-r.Height()-w.bottomScroll.Offset(), 0)
	end := min(start+r.Height(), len(lines))
	displayLines := lines[start:end]
	if len(displayLines) > 1 && !displayLines[0].author && displayLines[0].message.Author != "" {
		// Keep the sender visible when the viewport begins inside a long message.
		// Replace the oldest visible content row so pinning the author does not
		// discard the newest row at the bottom of the viewport.
		pinned := w.authorLine(displayLines[0].message, w.guildOf(w.active()))
		displayLines = append([]chatLine{pinned}, displayLines[1:]...)
	}
	w.visibleLines = append(w.visibleLines[:0], displayLines...)
	y := 0
	w.spinnerVisible = false
	drawnMedia := map[*inlineMedia]struct{}{}
	for _, line := range displayLines {
		if line.spinner {
			w.spinnerVisible = true
		}
		if line.media != nil {
			drawChatLine(r, 0, y, line)
			if _, ok := drawnMedia[line.media]; !ok {
				drawnMedia[line.media] = struct{}{}
				w.drawInlineMedia(r, line.mediaX, y-line.mediaRow, line.media, r.Width())
			}
			y++
			continue
		}
		drawChatLine(r, 0, y, line)
		for _, inline := range line.inlineMedia {
			w.drawInlineMedia(r, inline.col, y, inline.media, r.Width())
		}
		y++
	}
}

type chatLine struct {
	text        string
	style       screen.Style
	segments    []chatSegment
	message     store.Message
	author      bool
	media       *inlineMedia
	mediaRow    int
	mediaX      int
	inlineMedia []positionedInlineMedia
	actions     []componentHit
	entities    []entityHit
	// spinner marks a line that drew a media-loading spinner. Only spinners on
	// screen need the tick to animate them, so Draw tracks whether any visible
	// line carries this flag.
	spinner bool
}

type entityHit struct {
	start, end int
	action     markup.Action
}

type chatSegment struct {
	text  string
	style screen.Style
}

type inlineMedia struct {
	url          string
	label        string
	placementKey string
	cols         int
	rows         int
	img          image.Image
	style        screen.Style
}

type positionedInlineMedia struct {
	media *inlineMedia
	col   int
}

type chatMediaState struct {
	loading  bool
	img      image.Image
	err      error
	variants map[string]chatMediaVariant
	// rev increments whenever loading, img, or err changes. Cached message
	// bodies record the rev of every media state they read so they can be
	// invalidated when an image arrives or a state is evicted and refetched.
	rev uint32
	// touched is the render generation that last read this state, for sweeping.
	touched uint64
}

// mediaDep records the version of one media state a rendered body depended on.
type mediaDep struct {
	url string
	rev uint32
}

// chatCacheEntry is a memoized message body: everything render emits for a
// message except its author line, which depends on the preceding message and is
// cheap enough to recompute.
type chatCacheEntry struct {
	lines []chatLine
	// rev is the store revision of the message these lines were rendered from.
	// Comparing Message values instead would be silently wrong: the store hands
	// out shallow copies whose slices it patches in place.
	rev     uint64
	width   int
	channel store.ChannelID
	// metaRev and componentEpoch version the state a body reads but does not
	// own: resolved mentions and roles (store), and component presentation
	// (this widget).
	metaRev        uint64
	componentEpoch uint64
	deps           []mediaDep
	// gen is the render generation that last used this entry, for sweeping.
	gen uint64
}

type chatMediaVariant struct {
	img        image.Image
	cols, rows int
}

type mediaSpec struct {
	maxCols, maxRows int
	sourceW, sourceH int
	square           bool
}

func (s mediaSpec) key() string {
	return strconv.Itoa(s.maxCols) + "x" + strconv.Itoa(s.maxRows) + ":" +
		strconv.Itoa(s.sourceW) + "x" + strconv.Itoa(s.sourceH) + ":" +
		strconv.FormatBool(s.square)
}

func fitMediaCells(srcW, srcH, maxCols, maxRows int) (cols, rows int) {
	if srcW <= 0 {
		srcW = 1
	}
	if srcH <= 0 {
		srcH = 1
	}
	if maxCols <= 0 {
		maxCols = 1
	}
	if maxRows <= 0 {
		maxRows = 1
	}
	budgetH := maxRows * 2
	scaleW := float64(maxCols) / float64(srcW)
	scaleH := float64(budgetH) / float64(srcH)
	scale := minFloat(scaleW, scaleH)
	if scale > 1 {
		scale = 1
	}
	cols = max(min(int(float64(srcW)*scale), maxCols), 1)
	pixelsH := max(min(int(float64(srcH)*scale), budgetH), 1)
	rows = max(min((pixelsH+1)/2, maxRows), 1)
	return cols, rows
}

func mediaQuerySize(raw string) (width, height int, ok bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return 0, 0, false
	}
	q := u.Query()
	width = queryPositiveInt(q, "width")
	height = queryPositiveInt(q, "height")
	if width <= 0 && height <= 0 {
		if size := queryPositiveInt(q, "size"); size > 0 {
			width, height = size, size
		}
	}
	return width, height, width > 0 && height > 0
}

func queryPositiveInt(q url.Values, key string) int {
	value := q.Get(key)
	if value == "" {
		return 0
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// render turns the active channel's messages into wrapped, styled lines. Each
// message contributes a role-colored author line, its wrapped text content, and
// then any rich blocks: media chips, embeds, and a reactions line.
func (w *ChatView) render(width int) []chatLine {
	channel := w.active()
	guild := w.guildOf(channel)
	msgs := w.store.Messages(channel)
	w.renderGen++
	var lines []chatLine
	var previous store.Message
	previousSet := false
	for _, m := range msgs {
		// The author line depends on the preceding message, so it is not a pure
		// function of m and stays outside the cache. It is one concat and a
		// color lookup, so recomputing it is cheaper than tracking it.
		showAuthor := !previousSet || !sameMessageAuthor(previous, m) ||
			previous.Failed != m.Failed || previous.Pending != m.Pending
		if showAuthor {
			lines = append(lines, w.authorLine(m, guild))
		}
		body, ok := w.cachedBody(m, channel, width)
		if !ok {
			body = w.renderBody(m, channel, width)
		}
		lines = append(lines, body...)
		previous = m
		previousSet = true
	}
	w.sweepBodyCache()
	w.sweepMedia()
	return lines
}

// renderBody renders and caches everything a message contributes below its
// author line.
//
// The emoji placement counters are reset here, in the miss path, rather than
// once per message in render. Every placement key a body emits is prefixed with
// that message's own prefix and numbered from zero, so a body's keys depend
// only on the message — never on which neighbours were cache hits. That is what
// makes skipping a message safe.
func (w *ChatView) renderBody(m store.Message, channel store.ChannelID, width int) []chatLine {
	w.emojiKeyPrefix = messagePlacementPrefix(m)
	w.emojiSeq = 0
	style := w.styles.Text
	switch {
	case m.Failed:
		style = w.styles.Error
	case m.Pending:
		style = w.styles.Pending
	}

	w.bodyDeps = w.bodyDeps[:0]
	w.collectDeps = true

	var body []chatLine
	if m.Content != "" && !suppressContent(m) {
		body = append(body, stampMessage(w.renderContent(m.Content, width, style), m)...)
	}
	body = append(body, stampMessage(w.renderMedia(m, width, style), m)...)
	body = append(body, stampMessage(w.renderEmbeds(m, width, style), m)...)
	body = append(body, stampMessage(w.renderComponentTree(m, width, style), m)...)
	if line, ok := w.renderReactions(m.Reactions, messagePlacementPrefix(m)); ok {
		line.message = m
		body = append(body, line)
	}
	if line, ok := w.renderThreadStarter(m, channel); ok {
		body = append(body, line)
	}

	w.collectDeps = false
	w.storeBody(m, channel, width, body)
	return body
}

// cachedBody returns a previously rendered body when every input it depended on
// is unchanged.
func (w *ChatView) cachedBody(m store.Message, channel store.ChannelID, width int) ([]chatLine, bool) {
	e := w.bodyCache[messagePlacementPrefix(m)]
	if e == nil {
		return nil, false
	}
	if e.rev != m.Rev() || e.width != width || e.channel != channel ||
		e.metaRev != w.store.MetaRev() || e.componentEpoch != w.componentEpoch {
		return nil, false
	}
	for _, d := range e.deps {
		state := w.media[d.url]
		if state == nil || state.rev != d.rev {
			return nil, false
		}
	}
	// A hit skips renderBody, so ensureMedia never runs for this body. Mark its
	// media as still in use here or the sweep would evict it and every
	// subsequent frame would miss and re-render.
	for _, d := range e.deps {
		w.media[d.url].touched = w.renderGen
	}
	e.gen = w.renderGen
	return e.lines, true
}

// storeBody memoizes a rendered body, unless it drew a spinner. A loading body
// animates with w.spinner, which is deliberately not part of the cache key:
// including it would invalidate every entry twice a second and defeat the
// cache. Not caching the few loading bodies costs little and keeps the spinner
// moving.
func (w *ChatView) storeBody(m store.Message, channel store.ChannelID, width int, body []chatLine) {
	for _, d := range w.bodyDeps {
		if state := w.media[d.url]; state != nil && state.loading {
			return
		}
	}
	if w.bodyCache == nil {
		w.bodyCache = map[string]*chatCacheEntry{}
	}
	deps := append([]mediaDep(nil), w.bodyDeps...)
	w.bodyCache[messagePlacementPrefix(m)] = &chatCacheEntry{
		lines:          body,
		rev:            m.Rev(),
		width:          width,
		channel:        channel,
		metaRev:        w.store.MetaRev(),
		componentEpoch: w.componentEpoch,
		deps:           deps,
		gen:            w.renderGen,
	}
}

// maxBodyCache bounds the memoized bodies. It is comfortably above one
// channel's history so a single view never evicts its own entries, while still
// bounding a session that visits many channels.
const maxBodyCache = 600

// sweepBodyCache drops entries no recent render touched. Entries for other
// channels survive until the budget is reached, so flipping between two
// channels stays free.
func (w *ChatView) sweepBodyCache() {
	if len(w.bodyCache) <= maxBodyCache {
		return
	}
	for key, e := range w.bodyCache {
		if e.gen != w.renderGen {
			delete(w.bodyCache, key)
		}
	}
}

// maxMediaStates bounds the decoded images held per view. The media package's
// own LRU already caches decodes, so evicting here costs at most a cheap refetch
// that hits that LRU or the disk cache.
const maxMediaStates = 256

// sweepMedia drops media no recent render read. Without it w.media grows for
// the lifetime of the session, holding a decoded image for every URL seen in
// every channel visited.
//
// Evicting is safe for the body cache: a cached body whose media disappears
// fails its dependency check and re-renders. In-flight fetches are kept because
// their goroutine still expects the state it incremented the loading count for.
func (w *ChatView) sweepMedia() {
	if len(w.media) <= maxMediaStates {
		return
	}
	for url, state := range w.media {
		if state.touched != w.renderGen && !state.loading {
			delete(w.media, url)
		}
	}
}

// invalidateBodies drops every memoized body. Used when presentation state
// changes in a way that is not worth versioning precisely.
func (w *ChatView) invalidateBodies() {
	w.componentEpoch++
}

func (w *ChatView) authorLine(m store.Message, guild store.GuildID) chatLine {
	header := m.Author
	if m.Failed {
		header += " (failed)"
	} else if m.Pending {
		header += " (sending…)"
	}
	authorStyle := w.styles.Accent
	if color := w.store.MemberColor(guild, m.AuthorID); color != 0 {
		authorStyle.Fg = rgbColor(color)
	}
	return chatLine{text: header, style: authorStyle, message: m, author: true}
}

func sameMessageAuthor(a, b store.Message) bool {
	if a.AuthorID != 0 || b.AuthorID != 0 {
		return a.AuthorID != 0 && a.AuthorID == b.AuthorID
	}
	return a.Author == b.Author
}

// renderThreadStarter emits a "⤷ thread-name (N messages)" line under a message
// that started a thread. Discord gives a message-anchored thread the same
// snowflake as its anchor message, so the thread is found by the message ID.
func (w *ChatView) renderThreadStarter(m store.Message, channel store.ChannelID) (chatLine, bool) {
	c, ok := w.store.Channel(store.ChannelID(m.ID))
	if !ok || c.Kind != store.ChannelThread || c.ParentID != channel {
		return chatLine{}, false
	}
	text := "  ⤷ " + c.Name
	if c.Thread != nil && c.Thread.MessageCount > 0 {
		text += " (" + strconv.Itoa(c.Thread.MessageCount) + " messages)"
	}
	return chatLine{text: text, style: w.styles.Muted, message: m}, true
}

func stampMessage(lines []chatLine, m store.Message) []chatLine {
	for i := range lines {
		lines[i].message = m
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
	var entities []entityHit
	var inline []positionedInlineMedia
	spinner := false
	used := 0
	flush := func() {
		lines = append(lines, chatLine{segments: line, inlineMedia: inline, entities: entities, spinner: spinner})
		line = nil
		entities = nil
		inline = nil
		spinner = false
		used = 0
	}
	appendText := func(s string, style screen.Style, action *markup.Action) {
		parts := strings.Split(s, "\n")
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
				if action != nil {
					entities = append(entities, entityHit{start: used, end: used + cluster.Width, action: *action})
				}
				used += cluster.Width
			}
		}
	}
	for _, span := range markup.Parse(content, res) {
		style := w.markupStyle(span, base)
		if span.FG != 0 {
			style.Fg = rgbColor(span.FG)
		}
		if span.Kind == markup.Kind_FakeSticker {
			if media.ClassifyURL(span.URL) != media.ClassSticker {
				appendText(span.Text, style, nil)
				continue
			}
			if len(line) > 0 {
				flush()
			}
			w.emojiSeq++
			lines = append(lines, w.mediaLines(
				span.URL,
				"[sticker: "+span.Text+"]",
				w.emojiKeyPrefix+":sticker:"+strconv.Itoa(w.emojiSeq)+":"+span.URL,
				style,
				stickerMediaSpec(width),
			)...)
			continue
		}

		emojiURL := ""
		switch {
		case span.Kind == markup.Kind_Emoji && span.EmojiID != 0:
			emojiURL = customEmojiURL(span)
		case span.Kind == markup.Kind_FakeEmoji && media.ClassifyURL(span.URL) == media.ClassEmoji:
			emojiURL = span.URL
		}
		if emojiURL != "" && w.mediaCfg.Enabled && w.mediaCfg.EmojiImages {
			const emojiCols = 2
			if width > 0 && used > 0 && used+emojiCols > width {
				flush()
			}
			state := w.ensureMedia(emojiURL)
			placeholder := strings.Repeat(" ", emojiCols)
			if state != nil && state.loading {
				placeholder = mediaSpinner(w.spinner) + " "
				spinner = true
			}
			line = appendChatSegment(line, chatSegment{text: placeholder, style: style})
			if state != nil && state.err == nil && state.img != nil {
				variant := w.mediaVariant(state, mediaSpec{
					maxCols: emojiCols,
					maxRows: 1,
					sourceW: 48,
					sourceH: 48,
					square:  true,
				})
				w.emojiSeq++
				inline = append(inline, positionedInlineMedia{
					col: used,
					media: &inlineMedia{
						url:          emojiURL,
						label:        span.Text,
						placementKey: w.emojiKeyPrefix + ":emoji:" + strconv.Itoa(w.emojiSeq) + ":" + emojiURL,
						cols:         emojiCols,
						rows:         1,
						img:          variant.img,
						style:        style,
					},
				})
			}
			used += emojiCols
			continue
		}
		if span.Kind == markup.Kind_FakeEmoji {
			appendText(":"+span.Text+":", style, nil)
			continue
		}
		appendText(span.Text, style, span.Action)
	}
	if len(line) > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

func customEmojiURL(span markup.Span) string {
	return customEmojiURLParts(span.EmojiID, strings.Trim(span.Text, ":"), span.EmojiAnimated)
}

func customEmojiURLParts(id uint64, name string, animated bool) string {
	ext := "webp"
	if animated {
		ext = "gif"
	}
	return "https://cdn.discordapp.com/emojis/" +
		strconv.FormatUint(id, 10) + "." + ext +
		"?size=48&name=" + url.QueryEscape(name) + "&lossless=true"
}

func (w *ChatView) markupStyle(span markup.Span, base screen.Style) screen.Style {
	style := base
	if span.Quoted {
		style = mergeStyle(style, w.styles.Muted)
		style.Bg = base.Bg
	}
	switch span.Kind {
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
		style.Bg = base.Bg
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
	if span.Format&markup.FormatBold != 0 {
		style.Attrs |= screen.Bold
	}
	if span.Format&markup.FormatItalic != 0 {
		style.Attrs |= screen.Italic
	}
	if span.Format&markup.FormatUnderline != 0 {
		style.Attrs |= screen.Underline
	}
	if span.Format&markup.FormatStrike != 0 {
		style.Attrs |= screen.Strike
	}
	if span.Format&markup.FormatSpoiler != 0 {
		style.Attrs |= screen.Reverse
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
	case input.TickEvent:
		// Only advance the spinner when one is actually on screen. Media
		// loading elsewhere — scrolled out of view, or in another channel —
		// must not force a redraw.
		visible := w.spinnerVisible
		if visible {
			w.spinner++
		}
		return w.expireComponentFlashes(time.Now()) || visible
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		if ev.Key == input.KeyEnter || (ev.Key == input.KeyRune && ev.Rune == ' ') {
			if w.submitActiveComponentPicker() {
				return true
			}
		}
		if shortcut, ok := componentShortcutRune(ev); ok {
			if w.activateShortcut(shortcut) {
				return true
			}
		}
		switch ev.Key {
		case input.KeyEsc:
			w.bottomScroll.SetOffset(0)
			return true
		case input.KeyUp:
			w.scrollUp()
			return true
		case input.KeyDown:
			w.scrollDown()
			return true
		}
	case input.MouseEvent:
		if ev.Kind == input.MousePress && ev.Btn == input.ButtonLeft {
			if w.activateAt(ev.X, ev.Y, ev.Mods&input.Shift != 0) {
				return true
			}
		}
		if ev.Kind == input.MousePress && ev.Btn == input.ButtonRight {
			if ev.Y >= 0 && ev.Y < len(w.visibleLines) {
				msg := w.visibleLines[ev.Y].message
				if msg.ID != 0 {
					w.contextMessage = msg
					w.contextMessageSet = true
				}
			}
			return false
		}
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

func componentShortcutRune(ev input.KeyEvent) (rune, bool) {
	if ev.Key != input.KeyRune {
		return 0, false
	}
	switch ev.Rune {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return ev.Rune, true
	case '&':
		return '1', true
	case 'é', 'É':
		return '2', true
	case '"':
		return '3', true
	case '\'':
		return '4', true
	case '(':
		return '5', true
	case '-':
		return '6', true
	case 'è', 'È':
		return '7', true
	case '_':
		return '8', true
	case 'ç', 'Ç':
		return '9', true
	case 'à', 'À':
		return '0', true
	default:
		return 0, false
	}
}

func (w *ChatView) activateShortcut(shortcut rune) bool {
	for _, line := range w.visibleLines {
		for _, hit := range line.actions {
			if hit.action.shortcut == shortcut {
				return w.setComponentAction(hit.action)
			}
		}
	}
	return false
}

// activateAt dispatches the component under (x, y). Shift-clicking an option of
// a single-select flips that control into multi mode: options toggle like
// checkboxes and the picker submits all checked values on Enter or refold.
// Discord strips min/max_values from incoming selects (arikawa never
// unmarshals them), so shift-click is the user's explicit multi override.
func (w *ChatView) activateAt(x, y int, shiftMulti bool) bool {
	if y < 0 || y >= len(w.visibleLines) {
		return false
	}
	for _, hit := range w.visibleLines[y].entities {
		if x >= hit.start && x < hit.end {
			w.entityAction = hit.action
			w.entityActionSet = true
			return true
		}
	}
	for _, hit := range w.visibleLines[y].actions {
		if x >= hit.start && x < hit.end {
			action := hit.action
			if shiftMulti && action.option && !action.multi && action.kind == store.ComponentSelect {
				w.enableComponentMulti(action)
				action.multi = true
			}
			return w.setComponentAction(action)
		}
	}
	return false
}

// TakeEntityAction returns a clicked user/role mention action.
func (w *ChatView) TakeEntityAction() (markup.Action, bool) {
	if w == nil || !w.entityActionSet {
		return markup.Action{}, false
	}
	a := w.entityAction
	w.entityActionSet = false
	return a, true
}

func (w *ChatView) enableComponentMulti(action componentAction) {
	if w.multiPickers == nil {
		w.multiPickers = map[string]bool{}
	}
	w.multiPickers[action.controlKey()] = true
	w.invalidateBodies()
}

func (w *ChatView) componentMultiEnabled(controlKey string) bool {
	return w.multiPickers[controlKey]
}

func (w *ChatView) setComponentAction(action componentAction) bool {
	if action.disabled {
		return false
	}
	// Every path below changes how the component draws: the flash, the
	// expansion, or the selection.
	w.invalidateBodies()
	key := action.key()
	if w.componentFlashes == nil {
		w.componentFlashes = map[string]time.Time{}
	}
	w.componentFlashes[key] = time.Now().Add(500 * time.Millisecond)
	if action.option {
		w.setComponentSelection(action)
	} else if action.expandable {
		if w.expandedComponents == nil {
			w.expandedComponents = map[string]bool{}
		}
		key := action.controlKey()
		if w.expandedComponents[key] {
			w.expandedComponents[key] = false
			return w.submitComponentPicker(action)
		}
		w.expandedComponents[key] = true
		w.activePicker = action
		w.activePickerSet = true
		return true
	}
	if action.option {
		w.activePicker = action
		w.activePickerSet = true
		if action.multi {
			return true
		}
		if w.expandedComponents != nil {
			w.expandedComponents[action.controlKey()] = false
		}
		return w.submitComponentPicker(action)
	}
	w.componentAction = ComponentAction{
		Shortcut: action.shortcut,
		CustomID: action.customID,
		Label:    action.label,
		Kind:     action.kind,
		RawType:  action.rawType,
		Value:    action.value,
		URL:      action.url,
		Message:  action.message,
	}
	w.componentActionSet = true
	return true
}

func (w *ChatView) setComponentSelection(action componentAction) {
	w.invalidateBodies()
	if w.selectedComponents == nil {
		w.selectedComponents = map[string]map[string]bool{}
	}
	key := action.controlKey()
	if !action.multi {
		w.selectedComponents[key] = map[string]bool{action.value: true}
		return
	}
	selected := w.selectedComponents[key]
	if selected == nil {
		selected = componentValuesMap(action.defaults)
		w.selectedComponents[key] = selected
	}
	if selected[action.value] {
		delete(selected, action.value)
	} else {
		selected[action.value] = true
	}
}

func (w *ChatView) submitActiveComponentPicker() bool {
	if !w.activePickerSet {
		return false
	}
	action := w.activePicker
	if w.expandedComponents != nil && !w.expandedComponents[action.controlKey()] {
		return false
	}
	if w.expandedComponents != nil {
		w.expandedComponents[action.controlKey()] = false
		w.invalidateBodies()
	}
	return w.submitComponentPicker(action)
}

func (w *ChatView) submitComponentPicker(action componentAction) bool {
	if action.disabled || (!action.expandable && !action.option) {
		return false
	}
	multi := action.multi || w.componentMultiEnabled(action.controlKey())
	values := w.componentSelectedValues(action)
	label := action.label
	if action.option && multi && action.controlLabel != "" {
		label = action.controlLabel
	}
	value := action.value
	if multi || !action.option {
		value = ""
		if len(values) == 1 {
			value = values[0]
		}
	}
	delete(w.multiPickers, action.controlKey())
	w.componentAction = ComponentAction{
		Shortcut: action.shortcut,
		CustomID: action.customID,
		Label:    label,
		Kind:     action.kind,
		RawType:  action.rawType,
		Value:    value,
		Values:   values,
		URL:      action.url,
		Message:  action.message,
	}
	w.componentActionSet = true
	return true
}

func (w *ChatView) componentSelectedValues(action componentAction) []string {
	selected, ok := w.selectedComponents[action.controlKey()]
	if !ok {
		selected = componentValuesMap(action.defaults)
	}
	var values []string
	seen := map[string]bool{}
	for _, opt := range action.options {
		value := componentOptionValue(opt)
		if selected[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	for value := range selected {
		if !seen[value] {
			values = append(values, value)
		}
	}
	if len(values) == 0 && action.option && action.value != "" && !action.multi {
		values = append(values, action.value)
	}
	return values
}

func componentValuesMap(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func (w *ChatView) expireComponentFlashes(now time.Time) bool {
	if len(w.componentFlashes) == 0 {
		return false
	}
	changed := false
	for key, until := range w.componentFlashes {
		if !now.Before(until) {
			delete(w.componentFlashes, key)
			changed = true
		}
	}
	if changed {
		w.invalidateBodies()
	}
	return changed
}

// TakeContextMessage returns the message most recently right-clicked during the
// current event bubble. It clears the pending value so one click opens one menu.
func (w *ChatView) TakeContextMessage() (store.Message, bool) {
	if w == nil || !w.contextMessageSet {
		return store.Message{}, false
	}
	msg := w.contextMessage
	w.contextMessage = store.Message{}
	w.contextMessageSet = false
	return msg, true
}

// TakeComponentAction returns the most recent button/select activation captured
// by mouse or numeric shortcut. Live Discord submission is handled above ChatView.
func (w *ChatView) TakeComponentAction() (ComponentAction, bool) {
	if w == nil || !w.componentActionSet {
		return ComponentAction{}, false
	}
	action := w.componentAction
	w.componentAction = ComponentAction{}
	w.componentActionSet = false
	return action, true
}

func (w *ChatView) scrollUp() {
	w.bottomScroll.SetOffset(w.bottomScroll.Offset() + 1)
	if w.onReachTop != nil {
		w.onReachTop()
	}
}

func (w *ChatView) scrollDown() {
	if w.bottomScroll.Offset() > 0 {
		w.bottomScroll.SetOffset(w.bottomScroll.Offset() - 1)
	}
}
