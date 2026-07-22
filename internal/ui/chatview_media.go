package ui

import (
	"awesomeProject/internal/markup"
	"awesomeProject/internal/media"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/widget"
	"context"
	"hash/fnv"
	"net/url"
	"strconv"
	"strings"
	"time"
)

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
	prefix := w.focusKey + ":"
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
	prefix := w.focusKey + ":"
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
		// permanently loading state. Record a missing dependency so the body cache
		// cannot preserve this fallback forever: completion of an in-flight fetch
		// triggers another render, which can then retry after the queue drains.
		w.recordMediaDep(url, &chatMediaState{})
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
		w.mediaEpoch++
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
	w.mediaEpoch++
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
// Cross-layer overlap (a popup or toast drawn over the image) is handled by
// buffer occlusion, not z — see screen.Buffer.SetLayer.
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
