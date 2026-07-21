package ui

import (
	"context"
	"hash/fnv"
	"image"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"awesomeProject/internal/config"
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

	visibleLines            []chatLine
	visibleStart            int
	contextMessage          store.Message
	contextMessageSet       bool
	componentAction         ComponentAction
	componentActionSet      bool
	entityAction            markup.Action
	entityActionSet         bool
	componentFlashes        map[string]time.Time
	expandedComponents      map[string]bool
	collapsedHeaders        map[string]bool
	focusedMessage          store.Message
	focusedMessageSet       bool
	focusedExplicit         bool
	keyboardFocused         bool
	vimNavigation           bool
	vimPendingG             bool
	vimStickOldest          bool
	mouseBreakpointTracking bool
	highlightFocusBlock     bool
	focusKey                string
	focusOrder              []string
	focusMessages           map[string]store.Message
	focusRanges             map[string]messageRange
	focusStops              []chatFocusStop
	focusStopKey            string
	focusStopIndex          int
	renderLineCount         int
	viewportHeight          int
	lastRenderWidth         int
	onMessageAction         func(rune, store.Message)
	onMessageCopy           func([]store.Message)
	// Inline video: onPlayVideo starts playback of a chat video in the given
	// absolute cell region; onStopVideo tears the current playback down. The
	// widget owns activation and the stop conditions; the Shell owns the player.
	onPlayVideo func(url string, region media.Rect)
	onStopVideo func()
	// onOpenMedia opens a loaded image/GIF frame in the full-screen viewer.
	onOpenMedia func(url string, img image.Image, frames []media.Frame)
	// requestRedraw forces a repaint (App.Invalidate). Media loads and delivered
	// GIF frames call it so a loaded image appears — and a GIF starts animating —
	// on the next loop turn instead of waiting for the ~500ms idle tick.
	requestRedraw func()
	// videoHits are the video blocks drawn last frame, for click/key activation.
	videoHits []videoHit
	// chatOriginX/Y are the last Draw region's absolute top-left, used to turn a
	// chat-local video rect into absolute terminal cells for the player.
	chatOriginX int
	chatOriginY int
	// playingVideo is the URL of the video currently playing (blanked in Draw so
	// mpv's frames are not fought by the cell diff). videoSnap* capture the layout
	// at play time; any change stops playback so mpv never renders in stale cells.
	playingVideo       string
	videoSnapChannel   store.ChannelID
	videoSnapWidth     int
	videoSnapScroll    int
	selectionStart     int
	selectionActive    bool
	headerMessageKey   string
	headerSeq          int
	selectedComponents map[string]map[string]bool
	multiPickers       map[string]bool
	activePicker       componentAction
	activePickerSet    bool

	mediaCfg     media.Config
	mediaFetcher *media.Fetcher
	post         func(func())
	media        map[string]*chatMediaState
	mediaCtx     context.Context
	mediaCancel  context.CancelFunc
	mediaJobs    chan chatMediaJob
	mediaWG      sync.WaitGroup
	spinner      int
	// mediaLoadingCount tracks in-flight fetches so mediaLoading stays O(1)
	// as w.media grows.
	mediaLoadingCount int
	// spinnerVisible reports whether the last Draw put a spinner on screen.
	// Animating one that is scrolled out of view, or in another channel, would
	// invalidate the frame twice a second for nothing.
	spinnerVisible bool
	// animatedVisible reports whether the last Draw put a multi-frame GIF on
	// screen. It raises the tick cadence (via Animating) only while one is
	// visible, so off-screen and other-channel GIFs cost nothing.
	animatedVisible        bool
	roleGradients          bool
	roleGradientAnimations bool
	roleGradientPhase      float64
	roleGradientVisible    bool

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

	// stickyAnchor re-anchors a scrolled viewport to the message at its top
	// whenever lines elsewhere change height (folds, async media, unfurls).
	// BottomScroll alone assumes all growth is appended below the viewport,
	// which teleports the view when content above it grows or shrinks.
	stickyAnchor bool
	// anchorKey/anchorIntra identify the top visible line of the previous
	// draw: the placement prefix of its message and the line's index within
	// that message's block. anchorOffset is the scroll offset that draw used;
	// a differing offset on the next draw means the user scrolled, so the
	// stale anchor must not override their position.
	anchorKey    string
	anchorIntra  int
	anchorDelta  int
	anchorOffset int
	anchorSet    bool
	// pendingAnchor* pin a just-toggled fold control to its screen row for the
	// next draw (see chat_anchor.go).
	pendingAnchorKind uint8
	pendingAnchorKey  string
	pendingAnchorRow  int

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
	// Cells is the semantic cell palette. Legacy fields remain as compatibility
	// aliases for widgets that have not yet moved to a named surface.
	Cells     map[string]screen.Style
	Custom    map[string]bool
	Overrides *config.ColorOverrides
}

// Cell returns a semantic cell style, falling back to the legacy palette for
// callers that use a surface not present in Cells.
func (s Styles) Cell(name string) screen.Style {
	if style, ok := s.Cells[name]; ok {
		return config.ApplyColorRule(style, s.Overrides.Resolve(name))
	}
	var style screen.Style
	switch name {
	case "text", "messages.content":
		style = s.Text
	case "muted", "messages.thread", "messages.quote", "messages.code", "messages.attachment", "messages.reaction", "messages.timestamp":
		style = s.Muted
	case "messages.small":
		style = s.Muted
	case "accent", "messages.author", "messages.mention", "messages.roleMention", "messages.link", "messages.link.prettyLink", "messages.link.channel", "messages.link.message", "messages.link.invite":
		style = s.Accent
	case "messages.header1", "messages.header2", "messages.header3", "messages.header4", "messages.header5", "messages.header6":
		style := s.Accent
		style.Attrs |= screen.Bold | screen.Underline
		return config.ApplyColorRule(style, s.Overrides.Resolve(name))
	case "border", "panels.border":
		style = s.Border
	case "pending", "messages.pending":
		style = s.Pending
	case "error", "messages.failed":
		style = s.Error
	case "messages.focused":
		style = screen.Style{Attrs: screen.Reverse}
	case "messages.bold":
		style = screen.Style{Attrs: screen.Bold}
	case "messages.italic":
		style = screen.Style{Attrs: screen.Italic}
	case "messages.underlined":
		style = screen.Style{Attrs: screen.Underline}
	case "messages.strikethrough":
		style = screen.Style{Attrs: screen.Strike}
	case "messages.spoiler":
		style = screen.Style{Attrs: screen.Reverse}
	default:
		style = s.Text
	}
	return config.ApplyColorRule(style, s.Overrides.Resolve(name))
}

func (s Styles) HasCustom(name string) bool {
	return s.Custom[name] || s.Overrides.HasOverride(name)
}

// NewChatView returns a chat view over st. active reports which channel to show;
// resolver (optional) resolves mentions and channel references for markup.
func NewChatView(st *store.Store, active func() store.ChannelID, resolver func() markup.Resolver, styles Styles) *ChatView {
	return &ChatView{
		store:           st,
		active:          active,
		resolver:        resolver,
		styles:          styles,
		keyboardFocused: true,
		focusStopIndex:  -1,
		mediaCfg:        media.DefaultConfig(),
		node:            layout.Node{Grow: 1},
		stickyAnchor:    true,
	}
}

// OnReachTop registers a callback invoked when the user scrolls toward older
// messages. The callback runs on the UI goroutine.
func (w *ChatView) OnReachTop(fn func()) {
	if w != nil {
		w.onReachTop = fn
	}
}

// SetSource rebinds the transcript to a different account's store and
// active-channel accessor (used on multi-account switch) and resets the
// interactive view state — scroll, focus, and selection — so nothing from the
// previous account's messages leaks into the newly shown one. URL-keyed media
// state is left intact since attachment URLs are global.
func (w *ChatView) SetSource(st *store.Store, active func() store.ChannelID) {
	if w == nil {
		return
	}
	w.store = st
	w.active = active
	w.bottomScroll.SetOffset(0)
	w.focusedMessageSet = false
	w.focusedExplicit = false
	w.focusKey = ""
	w.focusStopKey = ""
	w.focusStopIndex = -1
	w.selectionActive = false
	w.selectionStart = 0
	w.contextMessageSet = false
	w.headerMessageKey = ""
	w.vimPendingG = false
}

// SetMedia enables asynchronous inline media loading for attachments, stickers,
// emoji CDN links, and image embeds. post must schedule callbacks on the UI
// goroutine; passing nil leaves text-chip fallbacks in place.
func (w *ChatView) SetMedia(fetcher *media.Fetcher, cfg media.Config, post func(func())) {
	if w == nil {
		return
	}
	if w.mediaCancel != nil {
		w.mediaCancel()
		w.mediaWG.Wait()
	}
	cfg = cfg.Bounded()
	w.mediaFetcher = fetcher
	w.mediaCfg = cfg
	w.post = post
	if w.media == nil {
		w.media = map[string]*chatMediaState{}
	}
	w.mediaCtx, w.mediaCancel = context.WithCancel(context.Background())
	w.mediaJobs = make(chan chatMediaJob, cfg.QueuedFetches)
	if fetcher == nil || post == nil || !cfg.Enabled {
		return
	}
	for range cfg.ConcurrentFetches {
		w.mediaWG.Add(1)
		go w.mediaWorker(w.mediaCtx, w.mediaJobs)
	}
}

// CloseMedia cancels queued and in-flight chat media requests. Workers use the
// same context for queue waits and HTTP, so none can remain blocked behind a
// semaphore after the view is closed.
func (w *ChatView) CloseMedia() {
	if w != nil && w.mediaCancel != nil {
		w.mediaCancel()
		w.mediaCancel = nil
		w.mediaWG.Wait()
	}
}

// SetRoleGradients opts author names into cached Discord role gradients. The
// animation option only repaints while a gradient author is visible.
func (w *ChatView) SetRoleGradients(enabled, animate bool) {
	if w == nil {
		return
	}
	w.roleGradients = enabled
	w.roleGradientAnimations = enabled && animate
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

// PreferredFocus starts an opted-in Vim session on message navigation rather
// than in the composer.
func (w *ChatView) PreferredFocus() bool { return w != nil && w.vimNavigation }

func (w *ChatView) VimFocusEnabled() bool { return w != nil && w.vimNavigation }

// SetFocusOwner records whether the chat panel itself owns keyboard focus.
// Component shortcut labels are deliberately hidden while another panel owns
// focus, preventing number keys typed elsewhere from looking actionable here.
func (w *ChatView) SetFocusOwner(focused bool) {
	if w == nil || w.keyboardFocused == focused {
		return
	}
	w.keyboardFocused = focused
	w.invalidateBodies()
}

// SetVimNavigation enables modal hjkl/message actions for this chat view.
// It is disabled by default so ordinary letter input remains inert outside
// explicit Vim configurations.
func (w *ChatView) SetVimNavigation(enabled bool) {
	if w != nil {
		w.vimNavigation = enabled
	}
}

// SetMouseBreakpointTracking opts pointer motion into changing the keyboard
// stopping point. Click activation remains available regardless of this flag.
func (w *ChatView) SetMouseBreakpointTracking(enabled bool) {
	if w != nil {
		w.mouseBreakpointTracking = enabled
	}
}

// SetHighlightFocusBlock expands the focused anchor across its full logical
// block, through the line before the next stopping point.
func (w *ChatView) SetHighlightFocusBlock(enabled bool) {
	if w != nil {
		w.highlightFocusBlock = enabled
	}
}

// OnMessageAction receives D/R/E/A actions for the focused message row.
func (w *ChatView) OnMessageAction(fn func(rune, store.Message)) { w.onMessageAction = fn }

// OnMessageCopy receives the messages selected through Vim visual mode.
func (w *ChatView) OnMessageCopy(fn func([]store.Message)) { w.onMessageCopy = fn }

// OnPlayVideo registers the callback that starts inline video playback. The
// region is in absolute terminal cells.
func (w *ChatView) OnPlayVideo(fn func(url string, region media.Rect)) { w.onPlayVideo = fn }

// OnStopVideo registers the callback that tears down the current playback.
func (w *ChatView) OnStopVideo(fn func()) { w.onStopVideo = fn }

// OnOpenMedia registers the callback that opens an image/GIF in the viewer.
func (w *ChatView) OnOpenMedia(fn func(url string, img image.Image, frames []media.Frame)) {
	w.onOpenMedia = fn
}

// SetInvalidate registers the repaint hook used to surface loaded media promptly.
func (w *ChatView) SetInvalidate(fn func()) { w.requestRedraw = fn }

func (w *ChatView) invalidate() {
	if w.requestRedraw != nil {
		w.requestRedraw()
	}
}

// SetPlayingVideo marks url as the video now playing so Draw reserves (blanks)
// its region for mpv. An empty url clears the mark. It snapshots the layout so
// any later change can stop playback before the region moves.
func (w *ChatView) SetPlayingVideo(url string) {
	if w == nil {
		return
	}
	w.playingVideo = url
	w.videoSnapChannel = w.active()
	w.videoSnapWidth = w.lastRenderWidth
	w.videoSnapScroll = w.bottomScroll.Offset()
}

// stopVideoRequest ends playback if one is active and notifies the Shell. It is
// the single path both widget-side stop conditions and mpv's own exit run
// through.
func (w *ChatView) stopVideoRequest() {
	if w == nil || w.playingVideo == "" {
		return
	}
	w.playingVideo = ""
	if w.onStopVideo != nil {
		w.onStopVideo()
	}
}

// playVideoHit starts playback for a recorded video block, translating its
// chat-local rect into absolute terminal cells.
func (w *ChatView) playVideoHit(h videoHit) bool {
	if w.onPlayVideo == nil {
		return false
	}
	w.onPlayVideo(h.url, media.Rect{
		X:    w.chatOriginX + h.x,
		Y:    w.chatOriginY + h.y,
		Cols: h.cols,
		Rows: h.rows,
	})
	return true
}

// playFocusedVideo plays the first video in the focused message, if any. Media
// placement keys are "<messagePrefix>:<kind>:<index>:<url>", so a HasPrefix on
// the focused message's prefix identifies its blocks without parsing (the prefix
// itself may contain colons for pending messages).
func (w *ChatView) playFocusedVideo() bool {
	if !w.focusedMessageSet {
		return false
	}
	prefix := messagePlacementPrefix(w.focusedMessage) + ":"
	for _, h := range w.videoHits {
		if strings.HasPrefix(h.placementKey, prefix) {
			return w.playVideoHit(h)
		}
	}
	return false
}

// openFocusedMedia opens the focused message's media in the viewer: a video
// plays in the full-screen player, otherwise the first loaded image/GIF frame is
// shown enlarged.
func (w *ChatView) openFocusedMedia() bool {
	if !w.focusedMessageSet {
		return false
	}
	if w.playFocusedVideo() {
		return true
	}
	prefix := messagePlacementPrefix(w.focusedMessage) + ":"
	for _, line := range w.visibleLines {
		b := line.media
		if b == nil || b.video() || b.img == nil {
			continue
		}
		if strings.HasPrefix(b.placementKey, prefix) {
			if b.linkURL != "" {
				w.entityAction = markup.Action{Kind: markup.ActionOpenURL, Target: b.linkURL}
				w.entityActionSet = true
				return true
			}
			if w.onOpenMedia != nil {
				w.onOpenMedia(b.url, b.img, w.mediaFrames(b.url))
				return true
			}
		}
	}
	return false
}

// mediaFrames snapshots an animation for the viewer, whose playback cursor is
// independent from the inline GIF.
func (w *ChatView) mediaFrames(url string) []media.Frame {
	if w == nil || w.media[url] == nil || len(w.media[url].frames) < 2 {
		return nil
	}
	return append([]media.Frame(nil), w.media[url].frames...)
}

func (w *ChatView) mediaLines(url, label, placementKey string, base screen.Style, spec mediaSpec, animated bool) []chatLine {
	return w.mediaLinesVideo(url, "", label, placementKey, base, spec, animated)
}

// mediaLinesLink renders a non-playable embed thumbnail whose activation opens
// target in the system browser.
func (w *ChatView) mediaLinesLink(url, target, label, placementKey string, base screen.Style, spec mediaSpec, animated bool) []chatLine {
	lines := w.mediaLines(url, label, placementKey, base, spec, animated)
	for i := range lines {
		if lines[i].media != nil {
			lines[i].media.linkURL = target
		}
	}
	return lines
}

// mediaLinesVideo renders inline media, optionally as a playable video. videoURL
// marks the block a play target; url (when set) is the poster image. A video
// without a poster still reserves a placeholder region so it can be played.
func (w *ChatView) mediaLinesVideo(url, videoURL, label, placementKey string, base screen.Style, spec mediaSpec, animated bool) []chatLine {
	muted := mergeStyle(base, w.styles.Cell("messages.attachment"))
	if url == "" {
		if videoURL == "" {
			return []chatLine{{segments: []chatSegment{{text: label, style: muted}}}}
		}
		return w.videoPlaceholderLines(videoURL, placementKey, base, spec)
	}
	state := w.ensureMedia(url, animated)
	switch {
	case state == nil:
		return []chatLine{{segments: []chatSegment{{text: label, style: muted}}}}
	case state.err != nil:
		// A video whose poster failed still offers a placeholder to play from.
		if videoURL != "" {
			return w.videoPlaceholderLines(videoURL, placementKey, base, spec)
		}
		return []chatLine{{segments: []chatSegment{{text: label + " (failed)", style: muted}}}}
	case state.img == nil:
		lines := w.loadingPlaceholderLines(label, base, spec)
		if trailer, ok := w.mediaTrailerLine(url, label, muted); ok {
			lines = append(lines, trailer)
		}
		return lines
	default:
		variant := w.mediaVariant(state, spec)
		if placementKey == "" {
			placementKey = url
		}
		block := &inlineMedia{url: url, label: label, placementKey: placementKey, cols: variant.cols, rows: variant.rows, img: variant.img, style: base, animated: state.animated(), videoURL: videoURL}
		lines := make([]chatLine, variant.rows)
		for i := range lines {
			lines[i] = chatLine{media: block, mediaRow: i}
		}
		if trailer, ok := w.mediaTrailerLine(url, label, muted); ok {
			lines = append(lines, trailer)
		}
		return lines
	}
}

// mediaTrailerLine is the compact text row drawn immediately below GIF stills
// and video posters (terminal graphics have no native overlay layer, so the
// GIF state and video affordance live in a real row). The loading placeholder
// appends the same row so the block height cannot change when the image
// arrives.
func (w *ChatView) mediaTrailerLine(url, label string, muted screen.Style) (chatLine, bool) {
	switch media.ClassifyURL(url) {
	case media.ClassGIF:
		if !w.mediaCfg.Animate {
			return chatLine{segments: []chatSegment{{text: "[GIF] " + label, style: muted}}}, true
		}
	case media.ClassVideo:
		return chatLine{segments: []chatSegment{{text: label, style: muted}}}, true
	}
	return chatLine{}, false
}

// loadingPlaceholderLines renders the loading spinner while reserving the exact
// number of rows the loaded image will occupy. Reserving the height up front
// means the async load swaps in place instead of growing the message and
// shifting the reader's viewport. When the source size is unknown it falls back
// to a single spinner line.
func (w *ChatView) loadingPlaceholderLines(label string, base screen.Style, spec mediaSpec) []chatLine {
	muted := mergeStyle(base, w.styles.Cell("messages.attachment"))
	spinnerLine := chatLine{segments: []chatSegment{{text: label + " " + mediaSpinner(w.spinner), style: muted}}, spinner: true}
	rows := w.reservedMediaRows(spec)
	if rows <= 1 {
		return []chatLine{spinnerLine}
	}
	lines := make([]chatLine, rows)
	lines[0] = spinnerLine
	for i := 1; i < rows; i++ {
		lines[i] = chatLine{segments: []chatSegment{{style: muted}}, spinner: true}
	}
	return lines
}

// reservedMediaRows is the row count a loaded image of spec's source size will
// occupy, matching mediaVariant's fit so the placeholder and the image are the
// same height. Returns 1 when the source size is unknown.
func (w *ChatView) reservedMediaRows(spec mediaSpec) int {
	if spec.sourceW <= 0 || spec.sourceH <= 0 {
		return 1
	}
	spec = w.normalizeMediaSpec(spec)
	_, rows := fitMediaCells(spec.sourceW, spec.sourceH, spec.maxCols, spec.maxRows)
	return max(rows, 1)
}

// videoPlaceholderLines builds a play region for a video that has no poster
// image. It reserves rows sized from spec (defaulting to a 16:9 box) so mpv has
// somewhere to draw and the block can be clicked or played by key.
func (w *ChatView) videoPlaceholderLines(videoURL, placementKey string, base screen.Style, spec mediaSpec) []chatLine {
	if spec.sourceW <= 0 || spec.sourceH <= 0 {
		spec.sourceW, spec.sourceH = 16, 9
	}
	spec = w.normalizeMediaSpec(spec)
	cols, rows := fitMediaCells(spec.sourceW, spec.sourceH, spec.maxCols, spec.maxRows)
	if placementKey == "" {
		placementKey = videoURL
	}
	block := &inlineMedia{label: "video", placementKey: placementKey, cols: cols, rows: rows, style: base, videoURL: videoURL}
	lines := make([]chatLine, rows)
	for i := range lines {
		lines[i] = chatLine{media: block, mediaRow: i}
	}
	return lines
}

// ensureMedia returns the load state for url, starting an async fetch on first
// use. animated requests all frames of an animated GIF (subject to
// Config.Animate); it only matters on the first fetch for a URL.
func (w *ChatView) ensureMedia(url string, animated bool) *chatMediaState {
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
	job := chatMediaJob{url: url, animated: animated}
	select {
	case w.mediaJobs <- job:
		w.media[url] = state
		w.mediaLoadingCount++
		w.recordMediaDep(url, state)
		return state
	default:
		// The bounded queue is saturated. Do not create a waiting goroutine or a
		// permanently loading state; a later render can retry after work drains.
		return nil
	}

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

type chatMediaJob struct {
	url      string
	animated bool
}

func (w *ChatView) mediaWorker(ctx context.Context, jobs <-chan chatMediaJob) {
	defer w.mediaWG.Done()
	for {
		// A canceled owner always wins over an already-buffered job.
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case job := <-jobs:
			if ctx.Err() != nil {
				return
			}
			w.fetchMedia(ctx, job.url, job.animated)
		}
	}
}

func (w *ChatView) fetchMedia(ctx context.Context, url string, animated bool) {
	if animated && w.mediaCfg.Animate {
		if frames, err := w.mediaFetcher.FetchGIF(ctx, url); err == nil && len(frames) > 0 {
			if len(frames) > maxAnimatedGIFFrames {
				frames = append([]media.Frame(nil), frames[:maxAnimatedGIFFrames]...)
			}
			frames = w.downscaleFrames(ctx, frames)
			if ctx.Err() == nil {
				w.post(func() {
					if ctx.Err() != nil {
						return
					}
					w.deliverFrames(url, frames)
					w.invalidate()
				})
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
	}
	img, err := w.mediaFetcher.Fetch(ctx, url)
	if ctx.Err() != nil {
		return
	}
	w.post(func() {
		if ctx.Err() != nil {
			return
		}
		state := w.media[url]
		if state == nil {
			return
		}
		if state.loading {
			w.mediaLoadingCount--
		}
		state.loading = false
		state.img = img
		state.err = err
		state.variants = nil
		state.rev++
		w.invalidate()
	})
}

// deliverFrames installs decoded GIF frames on the UI goroutine. A single-frame
// GIF is stored as a still image so it never drives the animator.
func (w *ChatView) deliverFrames(url string, frames []media.Frame) {
	state := w.media[url]
	if state == nil {
		state = &chatMediaState{}
		w.media[url] = state
	} else if state.loading {
		w.mediaLoadingCount--
	}
	state.loading = false
	state.err = nil
	state.img = frames[0].Image
	if len(frames) > 1 {
		state.frames = frames
	} else {
		state.frames = nil
	}
	state.frameIdx = 0
	state.frameElapsed = 0
	state.lastTick = time.Time{}
	state.variants = nil
	state.rev++
}

// downscaleFrames shrinks each frame to the fetcher's pixel budget. FetchGIF, unlike
// Fetch, does not downscale, so the animator would otherwise re-upload full-size
// frames every tick. Frames are freshly decoded per call, so mutating is safe.
func (w *ChatView) downscaleFrames(ctx context.Context, frames []media.Frame) []media.Frame {
	if w.mediaFetcher == nil {
		return frames
	}
	mp := w.mediaFetcher.MaxPixels
	if mp.X <= 0 || mp.Y <= 0 {
		return frames
	}
	for i := range frames {
		if ctx.Err() != nil {
			return nil
		}
		frames[i].Image = media.DownscaleToPixels(frames[i].Image, mp.X, mp.Y)
	}
	return frames
}

// gifDefaultFrameDelay clamps GIF frames that declare a zero (or absent) delay,
// matching how browsers treat such frames instead of spinning at full speed.
const gifDefaultFrameDelay = 100 * time.Millisecond

// advanceAnimations steps every visible animated GIF by the elapsed wall-clock
// time and reports whether any repointed to a new frame (and so needs a redraw).
func (w *ChatView) advanceAnimations() bool {
	now := time.Now()
	changed := false
	seen := make(map[*chatMediaState]struct{})
	for _, line := range w.visibleLines {
		if line.media == nil || !line.media.animated {
			continue
		}
		state := w.media[line.media.url]
		if !state.animated() {
			continue
		}
		if _, ok := seen[state]; ok {
			continue
		}
		seen[state] = struct{}{}
		if w.advanceFrames(state, now) {
			changed = true
		}
	}
	return changed
}

// advanceFrames advances one state's frame index by the time since the last
// advance, looping at the end. It returns whether the visible frame changed.
func (w *ChatView) advanceFrames(state *chatMediaState, now time.Time) bool {
	if state.lastTick.IsZero() {
		state.lastTick = now
		return false
	}
	dt := now.Sub(state.lastTick)
	state.lastTick = now
	if dt <= 0 {
		return false
	}
	if dt > time.Second {
		// The GIF was off-screen or the app was asleep; resync without a burst of
		// catch-up frames.
		state.frameElapsed = 0
		return false
	}
	state.frameElapsed += dt
	advanced := false
	for i := 0; i < len(state.frames); i++ {
		delay := state.frames[state.frameIdx].Delay
		if delay <= 0 {
			delay = gifDefaultFrameDelay
		}
		if state.frameElapsed < delay {
			break
		}
		state.frameElapsed -= delay
		state.frameIdx = (state.frameIdx + 1) % len(state.frames)
		advanced = true
	}
	if advanced {
		state.img = state.frames[state.frameIdx].Image
	}
	return advanced
}

// Animating reports whether a visible GIF needs the fast animation tick. The
// runtime reads it to raise the tick cadence only while something is moving.
func (w *ChatView) Animating() bool { return w != nil && w.animatedVisible }

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
			// Sizing is stable across frames, but the current frame is not; refresh
			// img so an animated GIF advances while reusing the cached cell fit.
			variant.img = state.img
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

func (w *ChatView) drawInlineMedia(r screen.Region, x, y int, block *inlineMedia, width int, focused bool) {
	if block == nil {
		return
	}
	if block.img == nil && !block.video() {
		return
	}
	cols := block.cols
	if cols <= 0 || x+cols > width {
		cols = max(min(width-x, block.cols), 1)
	}
	rows := max(block.rows, 1)
	// Kitty placements must remain below popup menus rendered over the chat.
	style := block.style
	if focused {
		style = w.styles.focusedStyle(style)
	}

	if block.video() {
		w.videoHits = append(w.videoHits, videoHit{x: x, y: y, cols: cols, rows: rows, url: block.videoURL, placementKey: block.placementKey})
		if block.videoURL == w.playingVideo {
			// mpv owns these cells while playing; keep them blank so the cell diff
			// leaves them for its frames instead of overwriting them.
			r.Fill(screen.Rect{X: x, Y: y, W: cols, H: rows}, screen.Cell{Content: " ", Style: style})
			return
		}
	}

	if block.img != nil {
		img := widget.NewKittyImageFrom(block.img).SetID(stableImageID(block.url)).SetZ(-1).SetStyle(style)
		if block.placementKey != "" {
			img.SetPlacementID(stableImageID(block.placementKey))
		}
		if b := block.img.Bounds(); b.Dx() > 0 && b.Dy() > 0 {
			img.SetPixelSize(b.Dx(), b.Dy())
		}
		img.Draw(r.Clip(screen.Rect{X: x, Y: y, W: cols, H: rows}))
	} else {
		// A posterless video draws a filled placeholder box as its play region.
		box := mergeStyle(style, w.styles.Cell("messages.attachment"))
		r.Fill(screen.Rect{X: x, Y: y, W: cols, H: rows}, screen.Cell{Content: " ", Style: box})
	}

	if block.video() {
		w.drawPlayGlyph(r, x, y, cols, rows, style)
	}
}

// drawPlayGlyph overlays a ▶ marker at the center of a video block. Inline
// images render below the text layer (z=-1), so the glyph stays visible on top.
func (w *ChatView) drawPlayGlyph(r screen.Region, x, y, cols, rows int, style screen.Style) {
	if cols <= 0 || rows <= 0 {
		return
	}
	s := style
	s.Attrs |= screen.Reverse
	r.Set(x+max((cols-1)/2, 0), y+rows/2, screen.Cell{Content: "▶", Style: s})
}

// stopVideoOnLayoutChange ends playback when the chat has relaid out in a way
// that would move mpv's region (channel switch, resize, or scroll), so mpv never
// renders into stale cells.
func (w *ChatView) stopVideoOnLayoutChange() {
	if w.playingVideo == "" {
		return
	}
	if w.active() != w.videoSnapChannel ||
		w.lastRenderWidth != w.videoSnapWidth ||
		w.bottomScroll.Offset() != w.videoSnapScroll {
		w.stopVideoRequest()
	}
}

// focusedStyle swaps a cell's colors by default. Explicit focused fg/bg rules
// are final colors from colors.conf, so they intentionally replace the swap.
func (s Styles) focusedStyle(base screen.Style) screen.Style {
	focused := s.Cell("messages.focused")
	if focused.Fg.Set() || focused.Bg.Set() {
		base.Attrs &^= screen.Reverse
		focused.Attrs &^= screen.Reverse
		return mergeStyle(base, focused)
	}
	base.Attrs |= screen.Reverse
	return mergeStyle(base, focused)
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
	fill(r, w.styles.Cell("messages.content"))
	if r.Width() <= 0 || r.Height() <= 0 {
		return
	}
	w.lastRenderWidth = r.Width()
	w.chatOriginX = r.Bounds().X
	w.chatOriginY = r.Bounds().Y
	w.stopVideoOnLayoutChange()
	w.ensureInitialFocusedMessage()
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
	preOffset := w.bottomScroll.Offset()
	if prepended {
		w.bottomScroll.UpdatePrepend(len(lines), r.Height())
	} else {
		w.bottomScroll.Update(len(lines), r.Height())
	}
	// A fold/unfold pins its toggled control line to the row it was activated
	// at; otherwise re-anchor a scrolled viewport onto the message that was at
	// its top last draw. The generic restore is skipped when the user is at
	// the bottom, scrolled since (the offset no longer matches the one the
	// anchor was captured under), or switched channels; and when the anchor
	// message left the transcript, BottomScroll's own adjustment stands.
	if w.applyPendingAnchor(lines, r.Height()) {
	} else if w.stickyAnchor && w.anchorSet && preOffset > 0 && preOffset == w.anchorOffset &&
		channel == w.lastMessageChannel {
		if idx, ok := anchorLineIndex(lines, w.anchorKey, w.anchorIntra); ok {
			w.bottomScroll.SetOffset(len(lines) - r.Height() - (idx - w.anchorDelta))
		}
	}
	w.buildFocusIndex(lines, r.Height())
	if w.lastMessageChannel != 0 && channel != w.lastMessageChannel {
		w.vimPendingG = false
		w.vimStickOldest = false
	}
	w.applyVimBoundaryFocus()
	w.lastMessageChannel = channel
	w.lastFirstMessage = firstMessage
	w.lastLastMessage = lastMessage
	// Bottom-align: show the last r.Height() lines, offset by scroll.
	start := max(len(lines)-r.Height()-w.bottomScroll.Offset(), 0)
	w.captureAnchor(lines, start)
	end := min(start+r.Height(), len(lines))
	displayLines := lines[start:end]
	stickyPinned := false
	if len(displayLines) > 1 && !displayLines[0].author && displayLines[0].message.Author != "" {
		// Keep the sender visible when the viewport begins inside a long message.
		// Replace the oldest visible content row so pinning the author does not
		// discard the newest row at the bottom of the viewport.
		pinned := w.authorLine(displayLines[0].message, w.guildOf(w.active()))
		displayLines = append([]chatLine{pinned}, displayLines[1:]...)
		stickyPinned = true
	}
	w.visibleLines = append(w.visibleLines[:0], displayLines...)
	w.visibleStart = start
	y := 0
	w.spinnerVisible = false
	w.animatedVisible = false
	w.roleGradientVisible = false
	w.videoHits = w.videoHits[:0]
	drawnMedia := map[*inlineMedia]struct{}{}
	for i, line := range displayLines {
		if line.roleGradient {
			w.roleGradientVisible = true
		}
		if line.spinner {
			w.spinnerVisible = true
		}
		if line.media != nil && line.media.animated {
			w.animatedVisible = true
		}
		lineIndex := start + i
		stop, focused, fillFocus := w.focusedHighlightAt(lineIndex)
		if w.selectionContainsLine(lineIndex) {
			focused, fillFocus = true, true
		}
		if !chatLineHasVisibleContent(line) {
			focused, fillFocus = false, false
		}
		focused = focused && !line.author
		focusStart, focusEnd := 0, r.Width()
		if focused && stop.kind == chatFocusControl && !fillFocus {
			focusStart, focusEnd = stop.start, stop.end
		}
		if line.media != nil {
			if focused {
				drawFocusedChatLine(r, 0, y, line, focusStart, focusEnd, w.styles.Cell("messages.focused"), fillFocus)
			} else {
				drawChatLine(r, 0, y, line)
			}
			if _, ok := drawnMedia[line.media]; !ok {
				drawnMedia[line.media] = struct{}{}
				mr := r
				if stickyPinned {
					// A media block whose top has scrolled above the viewport anchors
					// at y-mediaRow, which can fall on row 0 — the row now occupied by
					// the pinned author header. Clip it away from row 0 so its clear()
					// and image placement never erase the sticky author's name.
					mr = r.WithClip(screen.Rect{X: 0, Y: 1, W: r.Width(), H: max(r.Height()-1, 0)})
				}
				w.drawInlineMedia(mr, line.mediaX, y-line.mediaRow, line.media, r.Width(), focused)
			}
			y++
			continue
		}
		if focused {
			drawFocusedChatLine(r, 0, y, line, focusStart, focusEnd, w.styles.Cell("messages.focused"), fillFocus)
		} else {
			drawChatLine(r, 0, y, line)
		}
		for _, inline := range line.inlineMedia {
			w.drawInlineMedia(r, inline.col, y, inline.media, r.Width(), focused)
		}
		y++
	}
}

func (w *ChatView) ensureInitialFocusedMessage() {
	if w == nil || !w.keyboardFocused {
		return
	}
	messages := w.store.Messages(w.active())
	if len(messages) == 0 {
		return
	}
	latest := messages[len(messages)-1]
	if w.focusedMessageSet && (w.focusedExplicit || messagePlacementPrefix(w.focusedMessage) == messagePlacementPrefix(latest)) {
		return
	}
	if w.focusedMessageSet {
		w.invalidateBodies()
	}
	w.focusedMessage = latest
	w.focusedMessageSet = true
	w.focusKey = messagePlacementPrefix(w.focusedMessage)
	w.focusStopKey = ""
}

func (w *ChatView) buildFocusIndex(lines []chatLine, height int) {
	w.focusOrder = w.focusOrder[:0]
	if w.focusMessages == nil {
		w.focusMessages = map[string]store.Message{}
	}
	for key := range w.focusMessages {
		delete(w.focusMessages, key)
	}
	w.focusRanges = make(map[string]messageRange)
	firstBody := map[string]int{}
	for i, line := range lines {
		if line.message.ID == 0 && line.message.Nonce == "" {
			continue
		}
		key := messagePlacementPrefix(line.message)
		if !line.author {
			if _, ok := firstBody[key]; !ok {
				firstBody[key] = i
			}
		}
		if _, ok := w.focusMessages[key]; !ok {
			w.focusOrder = append(w.focusOrder, key)
			w.focusMessages[key] = line.message
			w.focusRanges[key] = messageRange{start: i, end: i + 1}
			continue
		}
		range_ := w.focusRanges[key]
		range_.end = i + 1
		w.focusRanges[key] = range_
	}
	w.focusStops = w.focusStops[:0]
	for i, line := range lines {
		if line.message.ID == 0 && line.message.Nonce == "" {
			continue
		}
		messageKey := messagePlacementPrefix(line.message)
		first, hasBody := firstBody[messageKey]
		if hasBody && (i == first || line.embedStart) {
			stop := chatFocusStop{kind: chatFocusMessage, key: messageKey + ":first", message: line.message, line: i}
			if line.embedKey != "" {
				stop.key = line.embedKey
			}
			if line.header != nil {
				stop.kind = chatFocusHeader
				stop.key = line.header.key
				stop.headerKey = line.header.key
			}
			w.focusStops = append(w.focusStops, stop)
		} else if line.header != nil {
			w.focusStops = append(w.focusStops, chatFocusStop{
				kind: chatFocusHeader, key: line.header.key, message: line.message,
				line: i, headerKey: line.header.key,
			})
		}
		for _, hit := range line.actions {
			w.focusStops = append(w.focusStops, chatFocusStop{
				kind: chatFocusControl, key: hit.action.key(), message: line.message,
				action: hit.action,
				line:   i, start: hit.start, end: hit.end,
			})
		}
	}
	w.renderLineCount = len(lines)
	w.viewportHeight = height
	if len(w.focusOrder) == 0 || len(w.focusStops) == 0 {
		w.focusKey = ""
		w.focusedMessageSet = false
		w.focusStopKey = ""
		w.focusStopIndex = -1
		return
	}
	selected := -1
	for i := range w.focusStops {
		if w.focusStops[i].key == w.focusStopKey {
			selected = i
			break
		}
	}
	if selected < 0 && w.focusedMessageSet {
		messageKey := messagePlacementPrefix(w.focusedMessage)
		for i := range w.focusStops {
			if messagePlacementPrefix(w.focusStops[i].message) == messageKey {
				selected = i
				break
			}
		}
	}
	if selected < 0 {
		selected = len(w.focusStops) - 1
	}
	w.focusStopIndex = selected
	w.focusStopKey = w.focusStops[selected].key
	w.focusedMessage = w.focusStops[selected].message
	w.focusKey = messagePlacementPrefix(w.focusedMessage)
	w.focusedMessageSet = true
}

type chatLine struct {
	text         string
	style        screen.Style
	segments     []chatSegment
	message      store.Message
	author       bool
	roleGradient bool
	embedStart   bool
	embedKey     string
	media        *inlineMedia
	mediaRow     int
	mediaX       int
	inlineMedia  []positionedInlineMedia
	actions      []componentHit
	entities     []entityHit
	header       *headerHit
	// spinner marks a line that drew a media-loading spinner. Only spinners on
	// screen need the tick to animate them, so Draw tracks whether any visible
	// line carries this flag.
	spinner bool
	// restrictHighlight keeps a focus or visual-selection highlight inside a
	// framed embed's content cells, leaving its box-drawing border intact.
	restrictHighlight bool
	highlightStart    int
	highlightEnd      int
}

type headerHit struct {
	key       string
	level     int
	collapsed bool
}

type chatFocusKind uint8

const (
	chatFocusMessage chatFocusKind = iota
	chatFocusHeader
	chatFocusControl
)

type chatFocusStop struct {
	kind      chatFocusKind
	key       string
	message   store.Message
	action    componentAction
	line      int
	start     int
	end       int
	headerKey string
}

type messageRange struct {
	start, end int
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
	// animated marks a multi-frame GIF so Draw flags it for the animation tick.
	animated bool
	// videoURL, when set, marks the block as a playable video. img (if any) is
	// the poster frame; a ▶ overlay invites activation. video without img draws a
	// placeholder box that still reserves the play region.
	videoURL string
	// linkURL makes an embed thumbnail open in the browser instead of the media
	// viewer. It is mutually exclusive with videoURL.
	linkURL string
}

// video reports whether this block is a playable video target.
func (m *inlineMedia) video() bool { return m != nil && m.videoURL != "" }

// videoHit records an on-screen video block for activation, in chat-local cells.
type videoHit struct {
	x, y         int
	cols         int
	rows         int
	url          string
	placementKey string
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
	// frames holds the decoded frames of an animated GIF. When it has more than
	// one frame the media is animated: the tick advances frameIdx and repoints
	// img at the current frame. nil (or a single frame) means a static image.
	frames []media.Frame
	// frameIdx is the frame img currently points at. frameElapsed accumulates
	// wall-clock time spent on it; lastTick timestamps the previous advance so
	// playback speed follows the real clock rather than the tick cadence.
	frameIdx     int
	frameElapsed time.Duration
	lastTick     time.Time
}

// animated reports whether the state holds a multi-frame animation.
func (s *chatMediaState) animated() bool { return s != nil && len(s.frames) > 1 }

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
	w.headerMessageKey = messagePlacementPrefix(m)
	w.headerSeq = 0
	style := w.styles.Cell("messages.content")
	switch {
	case m.Failed:
		style = w.styles.Cell("messages.failed")
	case m.Pending:
		style = w.styles.Cell("messages.pending")
	}

	w.bodyDeps = w.bodyDeps[:0]
	w.collectDeps = true

	var body []chatLine
	if m.Reply != nil {
		body = append(body, stampMessage([]chatLine{w.renderReplyLine(*m.Reply, channel, width)}, m)...)
	}
	if m.Content != "" && !suppressContent(m) {
		body = append(body, stampMessage(w.renderContent(m.Content, width, style), m)...)
	}
	body = append(body, stampMessage(w.renderForwards(m, width, style), m)...)
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
		// Loading bodies animate a spinner; animated bodies swap frames each tick.
		// Caching either would freeze that motion, so leave them uncached.
		if state := w.media[d.url]; state != nil && (state.loading || state.animated()) {
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

// maxAnimatedGIFFrames bounds per-placement retained frame memory. GIFs can
// contain thousands of frames; retaining the first loop is preferable to
// keeping an unbounded decoded animation alive in a long-running chat view.
const maxAnimatedGIFFrames = 120

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
	authorStyle := w.styles.Cell("messages.author")
	if m.Failed {
		authorStyle = mergeStyle(authorStyle, w.styles.Cell("messages.failed"))
	} else if m.Pending {
		authorStyle = mergeStyle(authorStyle, w.styles.Cell("messages.pending"))
	}
	if color := w.store.MemberColor(guild, m.AuthorID); color != 0 && !w.styles.HasCustom("messages.author") {
		authorStyle.Fg = rgbColor(color)
	}
	role, gradient := w.store.MemberDisplayRole(guild, m.AuthorID)
	gradient = gradient && role.Colors[1] != 0 && w.roleGradients && !w.styles.HasCustom("messages.author")
	avatarURL := m.AuthorAvatarURL
	if member, ok := w.store.Member(guild, m.AuthorID); ok && member.AvatarURL != "" {
		avatarURL = member.AvatarURL
	}
	if !gradient && (avatarURL == "" || !w.mediaCfg.Enabled) {
		return chatLine{text: header, style: authorStyle, message: m, author: true}
	}
	segments := []chatSegment(nil)
	if gradient {
		denominator := float64(len([]rune(header)))
		if denominator < 1 {
			denominator = 1
		}
		index := 0
		for cluster := range text.Clusters(header) {
			position := (float64(index) + w.roleGradientPhase) / denominator
			style := authorStyle
			style.Fg = rgbColor(role.GradientAt(position - float64(int(position))))
			segments = appendChatSegment(segments, chatSegment{text: cluster.Text, style: style})
			index++
		}
	}
	line := chatLine{
		segments:     segments,
		message:      m,
		author:       true,
		roleGradient: gradient,
	}
	if !gradient {
		line.segments = []chatSegment{{text: "  " + header, style: authorStyle}}
	} else if avatarURL != "" && w.mediaCfg.Enabled {
		line.segments = append([]chatSegment{{text: "  ", style: authorStyle}}, line.segments...)
	}
	if avatarURL == "" || !w.mediaCfg.Enabled {
		return line
	}
	if state := w.ensureMedia(avatarURL, false); state != nil && state.err == nil && state.img != nil {
		variant := w.mediaVariant(state, mediaSpec{maxCols: 2, maxRows: 1, sourceW: 48, sourceH: 48, square: true})
		line.inlineMedia = []positionedInlineMedia{{
			col:   0,
			media: &inlineMedia{url: avatarURL, placementKey: messagePlacementPrefix(m) + ":avatar", cols: variant.cols, rows: variant.rows, img: variant.img, style: authorStyle},
		}}
	}
	return line
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
	return chatLine{text: text, style: w.styles.Cell("messages.thread"), message: m}, true
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
	collapsedLevel := 0
	skip := false
	skipHeaderNewline := false
	var pendingHeader *chatLine
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
				if skipHeaderNewline && len(line) == 0 && len(inline) == 0 {
					skipHeaderNewline = false
				} else {
					flush()
				}
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
		if pendingHeader != nil && span.Kind != markup.Kind_Header {
			if span.HeaderLevel > 0 {
				style := w.markupStyle(span, base)
				for cluster := range text.Clusters(span.Text) {
					if cluster.Width == 0 || (width > 0 && chatLineWidth(*pendingHeader)+cluster.Width > width) {
						continue
					}
					pendingHeader.segments = appendChatSegment(pendingHeader.segments, chatSegment{text: cluster.Text, style: style})
				}
				continue
			}
			lines = append(lines, *pendingHeader)
			pendingHeader = nil
		}
		if span.Kind == markup.Kind_Header {
			level := span.HeaderLevel
			key := w.headerMessageKey + ":header:" + strconv.Itoa(w.headerSeq)
			w.headerSeq++
			if collapsedLevel != 0 {
				if level > collapsedLevel {
					continue
				}
				collapsedLevel = 0
				skip = false
			}
			style := w.markupStyle(span, base)
			collapsed := w.collapsedHeaders[key]
			marker := "▾ "
			if collapsed {
				marker = "▸ "
			}
			if len(line) > 0 || len(inline) > 0 {
				flush()
			}
			headerLine := chatLine{header: &headerHit{key: key, level: level, collapsed: collapsed}}
			for cluster := range text.Clusters(text.Truncate(marker+span.Text, width, text.Ellipsis)) {
				if cluster.Width == 0 {
					continue
				}
				headerLine.segments = appendChatSegment(headerLine.segments, chatSegment{text: cluster.Text, style: style})
			}
			if span.Text == "" {
				pendingHeader = &headerLine
			} else {
				lines = append(lines, headerLine)
			}
			skipHeaderNewline = true
			skip = collapsed
			if collapsed {
				collapsedLevel = level
			}
			continue
		}
		if skip {
			continue
		}
		style := w.markupStyle(span, base)
		mentionStyle := ""
		if span.Kind == markup.Kind_RoleMention {
			mentionStyle = "messages.roleMention"
		} else if span.Kind == markup.Kind_Mention || span.Kind == markup.Kind_ChannelMention {
			mentionStyle = "messages.mention"
		}
		if span.FG != 0 && (mentionStyle == "" || !w.styles.HasCustom(mentionStyle)) {
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
				false,
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
			state := w.ensureMedia(emojiURL, false)
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
	if pendingHeader != nil {
		lines = append(lines, *pendingHeader)
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
		style = mergeStyle(style, w.styles.Cell("messages.quote"))
		style.Bg = base.Bg
	}
	switch span.Kind {
	case markup.Kind_Bold:
		style = mergeStyle(style, w.styles.Cell("messages.bold"))
	case markup.Kind_Italic:
		style = mergeStyle(style, w.styles.Cell("messages.italic"))
	case markup.Kind_Code, markup.Kind_CodeBlock:
		style = mergeStyle(style, w.styles.Cell("messages.code"))
	case markup.Kind_Underline:
		style = mergeStyle(style, w.styles.Cell("messages.underlined"))
	case markup.Kind_Strike:
		style = mergeStyle(style, w.styles.Cell("messages.strikethrough"))
	case markup.Kind_Spoiler:
		// No hover in a TUI, so mask the text as a reverse-video block.
		style = mergeStyle(style, w.styles.Cell("messages.spoiler"))
	case markup.Kind_Link:
		style = mergeStyle(style, w.styles.Cell("messages.link.prettyLink"))
	case markup.Kind_Quote:
		style = mergeStyle(style, w.styles.Cell("messages.quote"))
		style.Bg = base.Bg
	case markup.Kind_Header:
		level := span.HeaderLevel
		if level < 1 || level > 6 {
			level = 1
		}
		style = mergeStyle(style, w.styles.Cell("messages.header"+strconv.Itoa(level)))
	case markup.Kind_Small:
		style = mergeStyle(style, w.styles.Cell("messages.small"))
	case markup.Kind_Mention, markup.Kind_ChannelMention:
		style = mergeStyle(style, w.styles.Cell("messages.mention"))
	case markup.Kind_RoleMention:
		style = mergeStyle(style, w.styles.Cell("messages.roleMention"))
	case markup.Kind_MessageLink, markup.Kind_ChannelLink, markup.Kind_InviteLink:
		name := "messages.link.invite"
		if span.Kind == markup.Kind_MessageLink {
			name = "messages.link.message"
		} else if span.Kind == markup.Kind_ChannelLink {
			name = "messages.link.channel"
		}
		style = mergeStyle(style, w.styles.Cell(name))
	case markup.Kind_Timestamp:
		style = mergeStyle(style, w.styles.Cell("messages.timestamp"))
	}
	if span.HeaderLevel > 0 {
		level := span.HeaderLevel
		if level > 6 {
			level = 6
		}
		style = mergeStyle(style, w.styles.Cell("messages.header"+strconv.Itoa(level)))
	}
	if span.Format&markup.FormatBold != 0 {
		style = mergeStyle(style, w.styles.Cell("messages.bold"))
	}
	if span.Format&markup.FormatItalic != 0 {
		style = mergeStyle(style, w.styles.Cell("messages.italic"))
	}
	if span.Format&markup.FormatUnderline != 0 {
		style = mergeStyle(style, w.styles.Cell("messages.underlined"))
	}
	if span.Format&markup.FormatStrike != 0 {
		style = mergeStyle(style, w.styles.Cell("messages.strikethrough"))
	}
	if span.Format&markup.FormatSpoiler != 0 {
		style = mergeStyle(style, w.styles.Cell("messages.spoiler"))
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

func drawFocusedChatLine(r screen.Region, x, y int, line chatLine, focusStart, focusEnd int, focus screen.Style, fillFocus bool) {
	if line.restrictHighlight {
		focusStart = max(focusStart, line.highlightStart)
		focusEnd = min(focusEnd, line.highlightEnd)
	}
	segments := line.segments
	if len(segments) == 0 {
		segments = []chatSegment{{text: line.text, style: line.style}}
	}
	// Segmented lines, notably markdown headers, carry their semantic color on
	// the segment rather than chatLine.style. Use that style for the trailing
	// focus fill so the whole row shares the header color.
	focusBase := line.style
	if len(segments) > 0 {
		focusBase = segments[0].style
	}
	col := x
	for _, segment := range segments {
		for cluster := range text.Clusters(segment.text) {
			if cluster.Width <= 0 || col >= r.Width() {
				continue
			}
			style := segment.style
			if col < focusEnd && col+cluster.Width > focusStart {
				style = Styles{}.focusedStyle(style)
				if focus.Fg.Set() || focus.Bg.Set() {
					style = mergeStyle(style, focus)
					style.Attrs &^= screen.Reverse
				}
			}
			r.Set(col, y, screen.Cell{Content: cluster.Text, Style: style})
			col += cluster.Width
		}
	}
	if fillFocus {
		style := Styles{}.focusedStyle(focusBase)
		if focus.Fg.Set() || focus.Bg.Set() {
			style = mergeStyle(style, focus)
			style.Attrs &^= screen.Reverse
		}
		for col < min(focusEnd, r.Width()) {
			if col >= focusStart {
				r.Set(col, y, screen.Cell{Content: " ", Style: style})
			}
			col++
		}
	}
}

func (w *ChatView) selectionContainsLine(line int) bool {
	if w == nil || !w.selectionActive || w.selectionStart < 0 || w.focusStopIndex < 0 {
		return false
	}
	start, end := w.selectionStart, w.focusStopIndex
	if start > end {
		start, end = end, start
	}
	for i := start; i <= end && i < len(w.focusStops); i++ {
		range_, ok := w.focusRanges[messagePlacementPrefix(w.focusStops[i].message)]
		if ok && line >= range_.start && line < range_.end {
			return true
		}
	}
	return false
}

func (w *ChatView) focusedStopAt(line int) (chatFocusStop, bool) {
	if w == nil || !w.keyboardFocused || w.focusStopIndex < 0 || w.focusStopIndex >= len(w.focusStops) {
		return chatFocusStop{}, false
	}
	stop := w.focusStops[w.focusStopIndex]
	return stop, stop.line == line
}

func (w *ChatView) focusedHighlightAt(line int) (chatFocusStop, bool, bool) {
	stop, exact := w.focusedStopAt(line)
	if !w.highlightFocusBlock || w.focusStopIndex < 0 || w.focusStopIndex >= len(w.focusStops) {
		return stop, exact, false
	}
	stop = w.focusStops[w.focusStopIndex]
	if line < stop.line {
		return chatFocusStop{}, false, false
	}
	messageKey := messagePlacementPrefix(stop.message)
	end := w.focusRanges[messageKey].end
	for i := w.focusStopIndex + 1; i < len(w.focusStops); i++ {
		next := w.focusStops[i]
		if messagePlacementPrefix(next.message) != messageKey {
			break
		}
		if next.line > stop.line {
			end = min(end, next.line)
			break
		}
	}
	return stop, line < end, true
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
		animated := w.animatedVisible && w.advanceAnimations()
		roleGradient := w.roleGradientAnimations && w.roleGradientVisible
		if roleGradient {
			w.roleGradientPhase += 0.08
		}
		return w.expireComponentFlashes(time.Now()) || visible || animated || roleGradient
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		if ev.Key == input.KeyEnter || (ev.Key == input.KeyRune && ev.Rune == ' ') {
			if w.submitActiveComponentPicker() {
				return true
			}
		}
		// 'p' plays the focused message's video; 'o' opens its media (video →
		// player, image/GIF → enlarged viewer) in the full-screen overlay.
		if ev.Key == input.KeyRune && (ev.Rune == 'p' || ev.Rune == 'o') && w.keyboardFocused && w.focusedMessageSet {
			if w.playingVideo != "" {
				w.stopVideoRequest()
				return true
			}
			if ev.Rune == 'o' {
				if w.openFocusedMedia() {
					return true
				}
			} else if w.playFocusedVideo() {
				return true
			}
		}
		vimRune := ev.Key == input.KeyRune && ev.Mods&(input.Ctrl|input.Alt|input.Super) == 0
		if w.vimNavigation && !vimRune {
			w.vimPendingG = false
			w.vimStickOldest = false
		}
		if vimRune && w.vimNavigation {
			if ev.Rune == 'g' {
				if w.vimPendingG {
					w.vimPendingG = false
					w.scrollToOldest()
					return true
				}
				w.vimPendingG = true
				return true
			}
			w.vimPendingG = false
			w.vimStickOldest = false
			switch ev.Rune {
			case 'G':
				w.scrollToNewest()
				return true
			case 'V':
				if !w.keyboardFocused || w.focusStopIndex < 0 {
					return false
				}
				w.selectionActive = !w.selectionActive
				w.selectionStart = w.focusStopIndex
				return true
			case 'Y':
				if !w.keyboardFocused || !w.focusedMessageSet || w.onMessageCopy == nil {
					return false
				}
				w.onMessageCopy(w.selectedMessages())
				w.selectionActive = false
				return true
			case 'j':
				w.scrollDown()
				return true
			case 'k':
				w.scrollUp()
				return true
			case 'J':
				w.moveFocus(1)
				return true
			case 'K':
				w.moveFocus(-1)
				return true
			case '-':
				if w.foldFocusedHeader() {
					return true
				}
			case 'd', 'D', 'r', 'R', 'e', 'E', 'a', 'A', 'u', 'U':
				if w.keyboardFocused && w.focusedMessageSet && w.onMessageAction != nil {
					w.onMessageAction(unicode.ToLower(ev.Rune), w.focusedMessage)
					return true
				}
			}
		}
		if shortcut, ok := componentShortcutRune(ev); ok {
			if w.activateShortcut(shortcut) {
				return true
			}
		}
		switch ev.Key {
		case input.KeyEsc:
			if w.playingVideo != "" {
				w.stopVideoRequest()
				return true
			}
			w.activePickerSet = false
			w.activePicker = componentAction{}
			w.expandedComponents = nil
			w.bottomScroll.SetOffset(0)
			w.invalidateBodies()
			return true
		case input.KeyUp:
			w.moveFocus(-1)
			return true
		case input.KeyDown:
			w.moveFocus(1)
			return true
		case input.KeyLeft:
			if w.vimNavigation && w.moveComponent(-1) {
				return true
			}
		case input.KeyRight:
			if w.vimNavigation && w.moveComponent(1) {
				return true
			}
		case input.KeyPageUp:
			w.pageUp()
			return true
		case input.KeyPageDown:
			w.pageDown()
			return true
		}
	case input.MouseEvent:
		if ev.Kind == input.MouseMotion && w.mouseBreakpointTracking && ev.Y >= 0 && ev.Y < len(w.visibleLines) {
			w.focusAtVisible(ev.X, ev.Y)
			return false
		}
		if ev.Kind == input.MousePress && ev.Btn == input.ButtonLeft {
			w.focusAtVisible(ev.X, ev.Y)
			if ev.Y >= 0 && ev.Y < len(w.visibleLines) && w.visibleLines[ev.Y].header != nil && ev.X < 2 {
				hit := w.visibleLines[ev.Y].header
				if w.collapsedHeaders == nil {
					w.collapsedHeaders = map[string]bool{}
				}
				w.anchorHeaderToggle(hit.key, ev.Y)
				w.collapsedHeaders[hit.key] = !hit.collapsed
				w.invalidateBodies()
				return true
			}
			if w.activateAt(ev.X, ev.Y, ev.Mods&input.Shift != 0) {
				return true
			}
		}
		if ev.Kind == input.MousePress && ev.Btn == input.ButtonRight {
			if ev.Y >= 0 && ev.Y < len(w.visibleLines) {
				msg := w.visibleLines[ev.Y].message
				if msg.ID != 0 {
					w.focusAtVisible(ev.X, ev.Y)
					w.contextMessage = msg
					w.contextMessageSet = true
					w.focusedMessage = msg
					w.focusedMessageSet = true
					w.focusKey = messagePlacementPrefix(msg)
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

func (w *ChatView) selectedMessages() []store.Message {
	if w == nil || !w.focusedMessageSet {
		return nil
	}
	start, end := w.focusStopIndex, w.focusStopIndex
	if w.selectionActive && w.selectionStart >= 0 {
		start = w.selectionStart
	}
	if start > end {
		start, end = end, start
	}
	seen := make(map[string]struct{}, end-start+1)
	messages := make([]store.Message, 0, end-start+1)
	for i := start; i <= end && i < len(w.focusStops); i++ {
		message := w.focusStops[i].message
		key := messagePlacementPrefix(message)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		messages = append(messages, message)
	}
	return messages
}

func (w *ChatView) focusAtVisible(x, y int) {
	if w == nil || y < 0 || y >= len(w.visibleLines) {
		return
	}
	if w.visibleLines[y].author {
		return
	}
	msg := w.visibleLines[y].message
	if msg.ID == 0 && msg.Nonce == "" {
		return
	}
	line := w.visibleStart + y
	selected := -1
	for i := range w.focusStops {
		stop := w.focusStops[i]
		if stop.line != line {
			continue
		}
		if stop.kind == chatFocusControl && x >= stop.start && x < stop.end {
			selected = i
			break
		}
		if selected < 0 {
			selected = i
		}
	}
	if selected < 0 {
		key := messagePlacementPrefix(msg)
		for i := range w.focusStops {
			if messagePlacementPrefix(w.focusStops[i].message) == key {
				selected = i
				break
			}
		}
	}
	if selected < 0 {
		return
	}
	previous := messagePlacementPrefix(w.focusedMessage)
	stop := w.focusStops[selected]
	w.focusStopIndex = selected
	w.focusStopKey = stop.key
	w.focusedMessage = stop.message
	w.focusedMessageSet = true
	w.focusedExplicit = true
	w.focusKey = messagePlacementPrefix(stop.message)
	if previous != w.focusKey {
		w.activePickerSet = false
		w.activePicker = componentAction{}
		w.invalidateBodies()
	}
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
	if w == nil || !w.keyboardFocused || !w.focusedMessageSet {
		return false
	}
	focused := messagePlacementPrefix(w.focusedMessage)
	for _, line := range w.visibleLines {
		for _, hit := range line.actions {
			if hit.action.shortcut == shortcut && messagePlacementPrefix(hit.action.message) == focused {
				return w.setComponentAction(hit.action)
			}
		}
	}
	return false
}

func (w *ChatView) foldFocusedHeader() bool {
	if w == nil || !w.keyboardFocused || w.focusStopIndex < 0 || w.focusStopIndex >= len(w.focusStops) {
		return false
	}
	stop := w.focusStops[w.focusStopIndex]
	if stop.kind != chatFocusHeader || stop.headerKey == "" {
		return false
	}
	if w.collapsedHeaders == nil {
		w.collapsedHeaders = map[string]bool{}
	}
	w.anchorHeaderToggle(stop.headerKey, stop.line-w.visibleStart)
	w.collapsedHeaders[stop.headerKey] = !w.collapsedHeaders[stop.headerKey]
	w.invalidateBodies()
	return true
}

// HandleVimFocus lets h/l move between adjacent components before falling back
// to the global focus ring. A collapsed header gets first refusal and unfolds
// in place, keeping the user inside the message instead of leaving the panel.
func (w *ChatView) HandleVimFocus(forward bool) bool {
	if w == nil || w.focusStopIndex < 0 || w.focusStopIndex >= len(w.focusStops) {
		return false
	}
	if w.vimNavigation && w.moveComponent(direction(forward)) {
		return true
	}
	stop := w.focusStops[w.focusStopIndex]
	if stop.kind != chatFocusHeader || stop.headerKey == "" || !w.collapsedHeaders[stop.headerKey] {
		return false
	}
	w.anchorHeaderToggle(stop.headerKey, stop.line-w.visibleStart)
	w.collapsedHeaders[stop.headerKey] = false
	w.invalidateBodies()
	return true
}

func direction(forward bool) int {
	if forward {
		return 1
	}
	return -1
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
	// A click inside a video block starts (or, if it is already the playing one,
	// stops) playback.
	for _, h := range w.videoHits {
		if x >= h.x && x < h.x+h.cols && y >= h.y && y < h.y+h.rows {
			if h.url == w.playingVideo {
				w.stopVideoRequest()
				return true
			}
			return w.playVideoHit(h)
		}
	}
	// A click on a loaded image/GIF block opens it enlarged in the viewer.
	if line := w.visibleLines[y]; line.media != nil && !line.media.video() && line.media.img != nil {
		if x >= line.mediaX && x < line.mediaX+line.media.cols {
			if line.media.linkURL != "" {
				w.entityAction = markup.Action{Kind: markup.ActionOpenURL, Target: line.media.linkURL}
				w.entityActionSet = true
				return true
			}
			if w.onOpenMedia != nil {
				w.onOpenMedia(line.media.url, line.media.img, w.mediaFrames(line.media.url))
				return true
			}
		}
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
		w.anchorControlToggle(key)
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
	w.vimStickOldest = false
	w.bottomScroll.SetOffset(w.bottomScroll.Offset() + 1)
	if w.onReachTop != nil {
		w.onReachTop()
	}
}

func (w *ChatView) moveFocus(delta int) {
	if len(w.focusStops) == 0 {
		if delta < 0 {
			w.scrollUp()
		} else {
			w.scrollDown()
		}
		return
	}
	index := w.focusStopIndex
	if index < 0 || index >= len(w.focusStops) {
		index = 0
	}
	next := index + delta
	if next < 0 || next >= len(w.focusStops) {
		if delta < 0 {
			w.scrollUp()
		} else {
			w.scrollDown()
		}
		return
	}
	w.setFocusStop(next)
	stop := w.focusStops[next]
	start := max(w.renderLineCount-w.viewportHeight-w.bottomScroll.Offset(), 0)
	end := min(start+w.viewportHeight, w.renderLineCount)
	if stop.line < start || stop.line >= end {
		w.bottomScroll.SetOffset(max(w.renderLineCount-w.viewportHeight-stop.line-1, 0))
	}
}

func (w *ChatView) setFocusStop(index int) {
	if w == nil || index < 0 || index >= len(w.focusStops) {
		return
	}
	previousMessage := messagePlacementPrefix(w.focusedMessage)
	stop := w.focusStops[index]
	w.focusStopIndex = index
	w.focusStopKey = stop.key
	w.focusedMessage = stop.message
	w.focusKey = messagePlacementPrefix(stop.message)
	w.focusedMessageSet = true
	w.focusedExplicit = true
	if previousMessage != w.focusKey {
		w.activePickerSet = false
		w.activePicker = componentAction{}
		w.invalidateBodies()
	}
}

func (w *ChatView) moveComponent(delta int) bool {
	if w == nil || delta == 0 || w.focusStopIndex < 0 || w.focusStopIndex >= len(w.focusStops) {
		return false
	}
	current := w.focusStops[w.focusStopIndex]
	messageKey := messagePlacementPrefix(current.message)
	for index := w.focusStopIndex + delta; index >= 0 && index < len(w.focusStops); index += delta {
		candidate := w.focusStops[index]
		if candidate.line != current.line || messagePlacementPrefix(candidate.message) != messageKey {
			return false
		}
		if candidate.kind == chatFocusControl {
			w.setFocusStop(index)
			return true
		}
	}
	return false
}

func (w *ChatView) pageUp() {
	w.vimStickOldest = false
	w.bottomScroll.SetOffset(w.bottomScroll.Offset() + max(w.viewportHeight, 1))
	if w.onReachTop != nil {
		w.onReachTop()
	}
}

func (w *ChatView) pageDown() {
	w.vimStickOldest = false
	w.bottomScroll.SetOffset(max(w.bottomScroll.Offset()-max(w.viewportHeight, 1), 0))
}

func (w *ChatView) scrollDown() {
	w.vimStickOldest = false
	if w.bottomScroll.Offset() > 0 {
		w.bottomScroll.SetOffset(w.bottomScroll.Offset() - 1)
	}
}

// scrollToOldest implements Vim's gg motion for the loaded transcript. Asking
// for older history at the boundary lets a subsequent gg continue toward the
// channel's true beginning when pagination has more messages available.
func (w *ChatView) scrollToOldest() {
	w.vimStickOldest = true
	w.bottomScroll.SetOffset(max(w.renderLineCount-w.viewportHeight, 0))
	if len(w.focusStops) > 0 {
		w.setFocusStop(0)
	}
	if w.onReachTop != nil {
		w.onReachTop()
	}
}

// scrollToNewest implements Vim's G motion.
func (w *ChatView) scrollToNewest() {
	w.vimStickOldest = false
	w.bottomScroll.SetOffset(0)
	if len(w.focusStops) > 0 {
		w.setFocusStop(len(w.focusStops) - 1)
	}
}

// applyVimBoundaryFocus keeps gg anchored to the true oldest loaded message
// when an asynchronous history page is prepended after the key sequence. It
// also moves the keyboard focus with the viewport, matching Vim's gg motion.
func (w *ChatView) applyVimBoundaryFocus() {
	if w == nil || !w.vimStickOldest {
		return
	}
	w.bottomScroll.SetOffset(max(w.renderLineCount-w.viewportHeight, 0))
	if len(w.focusStops) > 0 && w.focusStopIndex != 0 {
		w.setFocusStop(0)
	}
}
