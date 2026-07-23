package ui

import (
	"awesomeProject/internal/config"
	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
	"context"
	"image"
	"sync"
	"time"
	"unicode"
)

type ChatView struct {
	store       *store.Store
	active      func() store.ChannelID
	resolver    func() markup.Resolver
	onReachTop  func()
	styles      Styles
	borderChars widget.BorderChars
	node        layout.Node

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
	vimKeys                 config.VimKeys
	vimPendingG             bool
	vimStickOldest          bool
	mouseBreakpointTracking bool
	highlightFocusBlock     bool
	focusKey                string
	focusRanges             map[string]messageRange
	focusStops              []chatFocusStop
	focusStopKey            string
	focusStopIndex          int
	renderLineCount         int
	viewportHeight          int
	lastRenderWidth         int
	onMessageAction         func(rune, store.Message)
	onMessageCopy           func([]store.Message)
	onMessageFocus          func(store.Message)
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
	videoHits  []videoHit
	drawnMedia []*inlineMedia
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
	msgs        []store.Message
	transcript  chatTranscript
	focusGen    uint64
	mediaEpoch  uint64

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
	State     *StyleState
}

type StyleState struct {
	Generation uint64
}

// NewChatView returns a chat view over st. active reports which channel to show;
// resolver (optional) resolves mentions and channel references for markup.
func NewChatView(st *store.Store, active func() store.ChannelID, resolver func() markup.Resolver, styles Styles) *ChatView {
	return &ChatView{
		store:           st,
		active:          active,
		resolver:        resolver,
		styles:          styles,
		borderChars:     borderCharsForStyle("rounded"),
		keyboardFocused: true,
		vimKeys:         config.Default().Keys.Vim,
		focusStopIndex:  -1,
		mediaCfg:        media.DefaultConfig(),
		node:            layout.Node{Grow: 1},
		stickyAnchor:    true,
		msgs:            make([]store.Message, 0, store.DefaultHistoryLimit),
		bodyCache:       make(map[string]*chatCacheEntry, store.DefaultHistoryLimit),
		visibleLines:    make([]chatLine, 0, 64),
		drawnMedia:      make([]*inlineMedia, 0, 8),
		focusRanges:     make(map[string]messageRange, store.DefaultHistoryLimit),
		focusStops:      make([]chatFocusStop, 0, store.DefaultHistoryLimit*3),
	}
}

// SetBorderStyle selects the glyph set used to frame embeds and component
// containers. Unknown values retain the rounded default.
func (w *ChatView) SetBorderStyle(name string) {
	if w == nil {
		return
	}
	w.borderChars = borderCharsForStyle(name)
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

type chatMediaJob struct {
	url      string
	animated bool
}

const gifDefaultFrameDelay = 100 * time.Millisecond

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
	firstMessage, lastMessage := w.store.MsgEdges(channel)
	prepended := channel == w.lastMessageChannel &&
		w.lastFirstMessage != 0 && firstMessage != 0 &&
		firstMessage != w.lastFirstMessage && lastMessage == w.lastLastMessage
	preOffset := w.bottomScroll.Offset()
	if prepended {
		w.bottomScroll.UpdatePrepend(len(lines), r.Height())
	} else {
		// BottomScroll already distinguishes the two append states we need:
		// offset zero follows the live edge, while a non-zero offset grows with
		// appended lines to preserve the visible top. Do not derive the offset
		// from the previous visibleStart merely because a message is focused;
		// that would undo G/j/k changes and re-anchor unchanged draws.
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
		if idx, ok := w.anchorLineIndex(lines, w.anchorKey, w.anchorIntra); ok {
			w.bottomScroll.SetOffset(len(lines) - r.Height() - (idx - w.anchorDelta))
		}
	}
	if w.focusGen != w.transcript.gen {
		w.buildFocusIndex(lines, r.Height())
		w.focusGen = w.transcript.gen
	} else {
		w.renderLineCount = len(lines)
		w.viewportHeight = r.Height()
	}
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
	w.visibleLines = append(w.visibleLines[:0], lines[start:end]...)
	displayLines := w.visibleLines
	stickyPinned := false
	firstVisible := store.Message{}
	if len(displayLines) > 0 {
		firstVisible = w.msgAt(displayLines[0].msg)
	}
	if len(displayLines) > 1 && !displayLines[0].author && firstVisible.Author != "" {
		// Keep the sender visible when the viewport begins inside a long message.
		// Replace the oldest visible content row so pinning the author does not
		// discard the newest row at the bottom of the viewport.
		pinned := w.authorLine(firstVisible, w.guildOf(w.active()))
		pinned.msg = displayLines[0].msg
		displayLines[0] = pinned
		stickyPinned = true
	}
	w.visibleStart = start
	y := 0
	w.spinnerVisible = false
	w.animatedVisible = false
	w.roleGradientVisible = false
	w.videoHits = w.videoHits[:0]
	w.drawnMedia = w.drawnMedia[:0]
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
			seen := false
			for _, drawn := range w.drawnMedia {
				if drawn == line.media {
					seen = true
					break
				}
			}
			if !seen {
				w.drawnMedia = append(w.drawnMedia, line.media)
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

type chatLine struct {
	segments       []chatSegment
	inlineMedia    []positionedInlineMedia
	actions        []componentHit
	entities       []entityHit
	style          screen.Style
	text           string
	embedKey       string
	media          *inlineMedia
	header         *headerHit
	mediaRow       int
	mediaX         int
	highlightStart int
	highlightEnd   int
	msg            uint32
	author         bool
	roleGradient   bool
	embedStart     bool
	// spinner marks a line that drew a media-loading spinner. Only spinners on
	// screen need the tick to animate them, so Draw tracks whether any visible
	// line carries this flag.
	spinner bool
	// restrictHighlight keeps a focus or visual-selection highlight inside a
	// framed embed's content cells, leaving its box-drawing border intact.
	restrictHighlight bool
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
	action     componentAction
	key        string
	messageKey string
	headerKey  string
	line       int
	start      int
	end        int
	msg        uint32
	kind       chatFocusKind
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
	style        screen.Style
	img          image.Image
	url          string
	label        string
	placementKey string
	videoURL     string
	linkURL      string
	cols         int
	rows         int
	// animated marks a multi-frame GIF so Draw flags it for the animation tick.
	animated bool
	// videoURL, when set, marks the block as a playable video. img (if any) is
	// the poster frame; a ▶ overlay invites activation. video without img draws a
	// placeholder box that still reserves the play region.
	// linkURL makes an embed thumbnail open in the browser instead of the media
	// viewer. It is mutually exclusive with videoURL.
}

// video reports whether this block is a playable video target.
func (m *inlineMedia) video() bool { return m != nil && m.videoURL != "" }

type videoHit struct {
	url          string
	placementKey string
	x, y         int
	cols         int
	rows         int
}

type positionedInlineMedia struct {
	media *inlineMedia
	col   int
}

type chatMediaState struct {
	frames   []media.Frame
	lastTick time.Time
	img      image.Image
	err      error
	variants map[string]chatMediaVariant
	// touched is the render generation that last read this state, for sweeping.
	touched uint64
	// frames holds the decoded frames of an animated GIF. When it has more than
	// one frame the media is animated: the tick advances frameIdx and repoints
	// img at the current frame. nil (or a single frame) means a static image.
	// frameIdx is the frame img currently points at. frameElapsed accumulates
	// wall-clock time spent on it; lastTick timestamps the previous advance so
	// playback speed follows the real clock rather than the tick cadence.
	frameElapsed time.Duration
	frameIdx     int
	rev          uint32
	loading      bool
}

// animated reports whether the state holds a multi-frame animation.
func (s *chatMediaState) animated() bool { return s != nil && len(s.frames) > 1 }

type mediaDep struct {
	url string
	rev uint32
}

type chatCacheEntry struct {
	lines []chatLine
	deps  []mediaDep
	// rev is the store revision of the message these lines were rendered from.
	// Comparing Message values instead would be silently wrong: the store hands
	// out shallow copies whose slices it patches in place.
	rev             uint64
	width           int
	channel         store.ChannelID
	metaRev         uint64
	styleGeneration uint64
	// gen is the render generation that last used this entry, for sweeping.
	gen uint64
}

type chatTranscript struct {
	lines          []chatLine
	channel        store.ChannelID
	msgRev         uint64
	metaRev        uint64
	componentEpoch uint64
	styleGen       uint64
	mediaEpoch     uint64
	gen            uint64
	width          int
	stable         bool
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

const maxBodyCache = 600

const maxMediaStates = 256

const maxAnimatedGIFFrames = 120

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
		if w.vimNavigation {
			if keyMatches(ev, w.vimKeys.JumpOldest) {
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
			switch {
			case keyMatches(ev, w.vimKeys.JumpNewest):
				w.scrollToNewest()
				return true
			case keyMatches(ev, w.vimKeys.Select):
				if !w.keyboardFocused || w.focusStopIndex < 0 {
					return false
				}
				w.selectionActive = !w.selectionActive
				w.selectionStart = w.focusStopIndex
				return true
			case keyMatches(ev, w.vimKeys.Copy):
				if !w.keyboardFocused || !w.focusedMessageSet || w.onMessageCopy == nil {
					return false
				}
				w.onMessageCopy(w.selectedMessages())
				w.selectionActive = false
				return true
			case keyMatches(ev, w.vimKeys.ScrollDown):
				w.scrollDown()
				return true
			case keyMatches(ev, w.vimKeys.ScrollUp):
				w.scrollUp()
				return true
			case keyMatches(ev, w.vimKeys.NextMessage):
				w.moveFocus(1)
				return true
			case keyMatches(ev, w.vimKeys.PrevMessage):
				w.moveFocus(-1)
				return true
			case keyMatches(ev, w.vimKeys.Fold):
				if w.foldFocusedHeader() {
					return true
				}
			case vimAct(ev, w.vimKeys.Delete), vimAct(ev, w.vimKeys.Reply), vimAct(ev, w.vimKeys.Edit), vimAct(ev, w.vimKeys.AddReaction), vimAct(ev, w.vimKeys.Profile):
				if w.keyboardFocused && w.focusedMessageSet && w.onMessageAction != nil {
					action := unicode.ToLower(ev.Rune)
					for _, candidate := range []struct {
						spec   string
						action rune
					}{
						{w.vimKeys.Delete, 'd'}, {w.vimKeys.Reply, 'r'}, {w.vimKeys.Edit, 'e'},
						{w.vimKeys.AddReaction, 'a'}, {w.vimKeys.Profile, 'u'},
					} {
						if vimAct(ev, candidate.spec) {
							action = candidate.action
							break
						}
					}
					w.onMessageAction(action, w.focusedMessage)
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
			w.focusedExplicit = false
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
				msg := w.msgAt(w.visibleLines[ev.Y].msg)
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
