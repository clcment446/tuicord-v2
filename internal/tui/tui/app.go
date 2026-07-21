package tui

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/term"
)

// Idle widgets need only a low-frequency heartbeat. The runtime switches to
// animationTickInterval while a visible widget reports active animation.
const tickInterval = 500 * time.Millisecond

// animationTickInterval is the faster cadence used while a widget reports it is
// animating (e.g. an inline GIF). The runtime raises the tick rate only for the
// duration of the animation, keeping the idle app at tickInterval.
const animationTickInterval = 50 * time.Millisecond

// Animator is an optional Widget capability. A root that implements it lets the
// runtime switch to animationTickInterval while Animating reports true.
type Animator interface {
	Animating() bool
}

// Option configures an App.
type Option func(*App)

// App is the runtime coordinator for a retained widget tree.
type App struct {
	mu           sync.Mutex
	root         Widget
	size         Size
	hits         HitIndex
	dirty        bool
	forceRepaint bool
	posts        []func()
	postsStopped bool
	wake         chan struct{}
	rawWrites    [][]byte
	rawStopped   bool
	theme        Theme
	escExits     int
	mouseOn      bool
	focusSplits  bool
	ttyColors    bool
	// pendingFocus retains a generation-scoped exact request for a widget that
	// became focusable during event handling but is not in the focus ring until
	// the next render (for example Vim i enabling the composer).
	pendingFocus *pendingFocusRequest

	// Focus owns keyboard focus traversal for the retained tree.
	Focus FocusManager
	// Drag owns pointer capture for draggable widgets.
	Drag DragManager
}

// New returns an App with default runtime state.
func New(opts ...Option) *App {
	a := &App{
		dirty:    true,
		wake:     make(chan struct{}, 1),
		escExits: 5,
		mouseOn:  true,
	}
	for _, opt := range opts {
		opt(a)
	}
	a.Focus.SetOnChange(a.focusChanged)
	return a
}

// pendingFocusRequest keeps both the requesting root and its opaque
// generation so a later render cannot apply work made stale in the meantime.
type pendingFocusRequest struct {
	owner   Widget
	request FocusRequest
}

// WithMouse controls whether the app dispatches mouse events and asks the
// terminal runtime to enable mouse reporting.
func WithMouse(enabled bool) Option {
	return func(a *App) { a.mouseOn = enabled }
}

// WithFocusableSplits controls whether split selectors enter the focus ring.
func WithFocusableSplits(enabled bool) Option {
	return func(a *App) { a.focusSplits = enabled }
}

// WithTTYColors restricts emitted UI colors to the terminal's standard
// 16-color palette.
func WithTTYColors(enabled bool) Option {
	return func(a *App) { a.ttyColors = enabled }
}

// Post schedules fn to run on the App event loop.
//
// Posted functions execute in FIFO order. They should be short and are allowed
// to mutate widgets because they run on the UI goroutine.
func (a *App) Post(fn func()) { _ = a.TryPost(fn) }

// TryPost schedules fn unless event-loop shutdown has begun. The boolean lets
// asynchronous owners release resources that would otherwise be stranded in a
// closure the UI can no longer execute.
func (a *App) TryPost(fn func()) bool {
	if a == nil || fn == nil {
		return false
	}
	a.mu.Lock()
	if a.postsStopped {
		a.mu.Unlock()
		return false
	}
	a.posts = append(a.posts, fn)
	a.mu.Unlock()
	a.signal()
	return true
}

func (a *App) stopPosts() {
	a.mu.Lock()
	a.postsStopped = true
	a.mu.Unlock()
}

// WriteRaw queues one indivisible raw terminal command/transmission. It never
// blocks a producer: when the small queue is full it drops the oldest complete
// item, never a fragment, preserving Kitty command integrity. Calls after the
// event loop shuts down are ignored.
func (a *App) WriteRaw(b []byte) {
	if a == nil || len(b) == 0 {
		return
	}
	command := append([]byte(nil), b...)
	a.mu.Lock()
	if a.rawStopped {
		a.mu.Unlock()
		return
	}
	const maxRawWrites = 8
	if len(a.rawWrites) >= maxRawWrites {
		copy(a.rawWrites, a.rawWrites[1:])
		a.rawWrites[len(a.rawWrites)-1] = command
	} else {
		a.rawWrites = append(a.rawWrites, command)
	}
	a.mu.Unlock()
	a.signal()
}

func (a *App) takeRaw() []byte {
	a.mu.Lock()
	if len(a.rawWrites) == 0 {
		a.mu.Unlock()
		return nil
	}
	b := a.rawWrites[0]
	copy(a.rawWrites, a.rawWrites[1:])
	a.rawWrites[len(a.rawWrites)-1] = nil
	a.rawWrites = a.rawWrites[:len(a.rawWrites)-1]
	more := len(a.rawWrites) > 0
	a.mu.Unlock()
	if more {
		a.signal()
	}
	return b
}

func (a *App) stopRawWrites() {
	a.mu.Lock()
	a.rawStopped = true
	a.rawWrites = nil
	a.mu.Unlock()
}

// Invalidate marks the current frame dirty so the next event-loop turn redraws.
func (a *App) Invalidate() {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.dirty = true
	a.mu.Unlock()
	a.signal()
}

// ForceRepaint schedules a full repaint that re-emits every cell and graphic on
// the next render, discarding the diff baseline. Use after external output (mpv
// video) has drawn over the screen so the widget tree cleanly repaints.
func (a *App) ForceRepaint() {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.forceRepaint = true
	a.dirty = true
	a.mu.Unlock()
	a.signal()
}

func (a *App) takeForceRepaint() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.forceRepaint
	a.forceRepaint = false
	return f
}

func (a *App) forceRepaintPending() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.forceRepaint
}

// Dirty reports whether the app currently needs to redraw.
func (a *App) Dirty() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dirty
}

// animating reports whether the root widget wants the faster animation tick.
func (a *App) animating() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	root := a.root
	a.mu.Unlock()
	if an, ok := root.(Animator); ok {
		return an.Animating()
	}
	return false
}

// Render draws root at size and refreshes the hit-test and focus indexes.
func (a *App) Render(root Widget, size Size) *screen.Buffer {
	// Keep root notification intact even for a zero-value App initialized by an
	// embedder instead of New.
	a.Focus.SetOnChange(a.focusChanged)
	if size.W < 0 {
		size.W = 0
	}
	if size.H < 0 {
		size.H = 0
	}
	buf := screen.NewBuffer(size.W, size.H)
	if bg := a.theme.Background; bg.Set() {
		buf.Fill(screen.Rect{W: size.W, H: size.H}, screen.Cell{Content: " ", Style: screen.Style{Bg: bg}})
	}
	// Publish the new root before rebuilding focus so every replacement change
	// is reported to the tree that owns the rendered interaction state.
	a.mu.Lock()
	a.root = root
	a.size = size
	a.mu.Unlock()
	if root == nil || size.W == 0 || size.H == 0 {
		a.discardPendingFocus(false)
		a.mu.Lock()
		a.hits = HitIndex{}
		a.dirty = false
		a.mu.Unlock()
		a.Focus.Clear()
		return buf
	}

	root.Measure(size)
	hits := BuildHitIndex(root, size)
	measureHits(hits)
	hits = BuildHitIndex(root, size)
	widgets := collectWidgets(root)
	applyFocusPolicy(widgets, a.focusSplits)
	// Focus only widgets with non-empty final visible layout. This excludes
	// hidden responsive panes and zero-area controls without changing the
	// retained tree used for indicators, overlays, or later larger renders.
	a.Focus.Replace(visibleWidgets(root, hits))
	a.applyPendingFocus(root)
	applyFocusIndicators(widgets, a.Focus.Focused())
	drawTree(buf, root, hits)

	a.mu.Lock()
	a.hits = hits
	a.dirty = false
	a.mu.Unlock()
	return buf
}

// Handle routes an input event through drag, focus, and mouse hit testing.
func (a *App) Handle(ev Event) bool {
	if a == nil || ev == nil {
		return false
	}
	switch ev := ev.(type) {
	case input.MouseEvent:
		if !a.mouseOn {
			return false
		}
		handled := a.handleMouse(ev)
		a.consumeFocusRequests()
		return handled
	case input.KeyEvent:
		handled := a.handleKey(ev)
		a.consumeFocusRequests()
		return handled
	case input.TickEvent:
		return a.handleTick(ev)
	default:
		handled := a.handleFocused(ev)
		a.consumeFocusRequests()
		return handled
	}
}

// handleTick broadcasts a tick to both the focused widget and the root. A tick
// is a synthetic timer event, not targeted input: the focused widget uses it
// for its own updates (e.g. cursor blink) but must not starve the root of it,
// or root-level time-based work like toast auto-dismiss would stall whenever a
// widget consuming ticks holds focus.
func (a *App) handleTick(ev Event) bool {
	handled := false
	focused := a.Focus.Focused()
	if focused != nil && focused.Handle(ev) {
		handled = true
	}
	a.mu.Lock()
	root := a.root
	a.mu.Unlock()
	if root != nil && !sameWidget(root, focused) && root.Handle(ev) {
		handled = true
	}
	return handled
}

// Run opens the user's terminal and blocks until the app exits.
func (a *App) Run(root Widget) error {
	return a.RunContext(context.Background(), root)
}

// RunContext opens the user's terminal and runs root until ctx is canceled,
// terminal input ends, or a terminal read/write error occurs.
func (a *App) RunContext(ctx context.Context, root Widget) error {
	if a == nil {
		a = New()
	}
	return term.RunWithOptions(term.Options{Mouse: a.mouseOn}, func(t *term.Terminal) error {
		reader := input.NewReader(ctx, t)
		sz, err := t.Size()
		if err != nil {
			return err
		}
		return a.run(ctx, root, t, reader.Events(), reader.Errors(), t.Resizes(), Size{W: sz.Width, H: sz.Height})
	})
}

func (a *App) handleMouse(ev input.MouseEvent) bool {
	a.mu.Lock()
	root := a.root
	a.mu.Unlock()
	if overlay, ok := root.(EventOverlay); ok && overlay.HandleOverlay(ev) {
		return true
	}
	a.mu.Lock()
	hits := a.hits
	a.mu.Unlock()

	if ev.Kind == input.MousePress {
		if hit, ok := hits.Hit(ev.X, ev.Y); ok {
			a.focusDeepest(hits.Path(ev.X, ev.Y), hit.Widget)
		}
	}
	if a.Drag.HandleMouse(ev, hits) {
		a.Invalidate()
		return true
	}
	path := hits.Path(ev.X, ev.Y)
	for i := len(path) - 1; i >= 0; i-- {
		if path[i].Widget.Handle(localMouse(ev, path[i])) {
			return true
		}
	}
	return false
}

func localMouse(ev input.MouseEvent, hit Hit) input.MouseEvent {
	ev.X = hit.X
	ev.Y = hit.Y
	return ev
}

func (a *App) handleKey(ev input.KeyEvent) bool {
	a.mu.Lock()
	root := a.root
	a.mu.Unlock()
	if overlay, ok := root.(EventOverlay); ok && overlay.HandleOverlay(ev) {
		return true
	}
	if a.Drag.Active() && ev.Key == input.KeyEsc && !ev.Release {
		a.Drag.Cancel()
		a.Invalidate()
		return true
	}
	if !ev.Release && ev.Mods&input.Alt != 0 && ev.Mods&(input.Ctrl|input.Super) == 0 {
		var focused Widget
		switch ev.Key {
		case input.KeyLeft:
			focused = a.Focus.Back()
		case input.KeyRight:
			focused = a.Focus.Forward()
		}
		if focused != nil {
			a.Invalidate()
			return true
		}
	}
	if !ev.Release && ev.Key == input.KeyRune && ev.Mods == 0 && (ev.Rune == 'h' || ev.Rune == 'l') {
		focused := a.Focus.Focused()
		if traverser, ok := focused.(VimFocusTraverser); ok && traverser.VimFocusEnabled() {
			forward := ev.Rune == 'l'
			if traverser.HandleVimFocus(forward) {
				a.Invalidate()
				return true
			}
			if forward {
				a.Focus.Next()
			} else {
				a.Focus.Prev()
			}
			a.Invalidate()
			return true
		}
	}
	if !ev.Release && ev.Key == input.KeyRune && ev.Mods == 0 && (ev.Rune == 'H' || ev.Rune == 'L') {
		if focused := a.Focus.Focused(); focused != nil {
			if traverser, ok := focused.(VimFocusTraverser); ok && traverser.VimFocusEnabled() {
				if ev.Rune == 'L' {
					a.Focus.Next()
				} else {
					a.Focus.Prev()
				}
				a.Invalidate()
				return true
			}
		}
	}
	if ev.Key == input.KeyTab && !ev.Release {
		// A retained modal may own an internal field cycle. Give the focused
		// widget and its ancestors first refusal before global ring traversal.
		if a.handleFocused(ev) {
			return true
		}
		if ev.Mods&input.Shift != 0 {
			a.Focus.Prev()
		} else {
			a.Focus.Next()
		}
		a.Invalidate()
		return true
	}
	return a.handleFocused(ev)
}

// consumeFocusRequests applies root-owned exact and configurable traversal
// requests after the event finishes bubbling.
func (a *App) consumeFocusRequests() {
	if a == nil {
		return
	}
	a.mu.Lock()
	root := a.root
	a.mu.Unlock()
	if requester, ok := root.(FocusRequester); ok {
		if requested, ready := requester.TakeFocusRequest(); ready && requested.Target != nil {
			a.installFocusRequest(root, requested)
		}
	}
	if requester, ok := root.(FocusTraversalRequester); ok {
		step := requester.TakeFocusTraversalRequest()
		requested := step
		for step > 0 {
			a.Focus.Next()
			step--
		}
		for step < 0 {
			a.Focus.Prev()
			step++
		}
		if requested != 0 {
			a.Invalidate()
		}
	}
}

func (a *App) installFocusRequest(owner Widget, request FocusRequest) {
	if !focusRequestValid(owner, request) {
		focusRequestDone(owner, request, false)
		return
	}
	if a.Focus.set(request.Target, FocusChangeDirect) {
		a.discardPendingFocus(false)
		focusRequestDone(owner, request, true)
	} else {
		a.mu.Lock()
		previous := a.pendingFocus
		a.pendingFocus = &pendingFocusRequest{owner: owner, request: request}
		a.mu.Unlock()
		if previous != nil {
			focusRequestDone(previous.owner, previous.request, false)
		}
	}
	a.Invalidate()
}

func (a *App) applyPendingFocus(root Widget) {
	a.mu.Lock()
	pending := a.pendingFocus
	a.mu.Unlock()
	if pending == nil {
		return
	}
	if !sameWidget(pending.owner, root) || !focusRequestValid(pending.owner, pending.request) {
		a.discardPendingFocus(false)
		return
	}
	if a.Focus.set(pending.request.Target, FocusChangeDirect) {
		a.discardPendingFocus(true)
	}
}

func (a *App) discardPendingFocus(applied bool) {
	if a == nil {
		return
	}
	a.mu.Lock()
	pending := a.pendingFocus
	a.pendingFocus = nil
	a.mu.Unlock()
	if pending != nil {
		focusRequestDone(pending.owner, pending.request, applied)
	}
}

func focusRequestValid(owner Widget, request FocusRequest) bool {
	if owner == nil || request.Target == nil {
		return false
	}
	validator, ok := owner.(FocusRequestValidator)
	return !ok || validator.FocusRequestValid(request)
}

func focusRequestDone(owner Widget, request FocusRequest, applied bool) {
	if completer, ok := owner.(FocusRequestCompleter); ok {
		completer.FocusRequestDone(request, applied)
	}
}

func (a *App) handleFocused(ev Event) bool {
	a.mu.Lock()
	root := a.root
	a.mu.Unlock()
	focused := a.Focus.Focused()
	if focused == nil {
		return root != nil && root.Handle(ev)
	}
	if focused.Handle(ev) {
		return true
	}
	path := widgetPath(root, focused)
	if len(path) == 0 {
		if root != nil && !sameWidget(root, focused) {
			return root.Handle(ev)
		}
		return false
	}
	for i := len(path) - 2; i >= 0; i-- {
		ancestor := path[i]
		if bubbler, ok := ancestor.(EventBubbler); ok {
			if bubbler.HandleBubble(ev) {
				return true
			}
			continue
		}
		if ancestor.Handle(ev) {
			return true
		}
	}
	return false
}

func widgetPath(root, target Widget) []Widget {
	if root == nil || target == nil {
		return nil
	}
	var path []Widget
	var walk func(Widget) bool
	walk = func(w Widget) bool {
		if w == nil {
			return false
		}
		path = append(path, w)
		if sameWidget(w, target) {
			return true
		}
		if container, ok := w.(Container); ok {
			for _, child := range container.Children() {
				if walk(child) {
					return true
				}
			}
		}
		path = path[:len(path)-1]
		return false
	}
	if !walk(root) {
		return nil
	}
	return path
}

func (a *App) focusDeepest(path []Hit, fallback Widget) {
	for i := len(path) - 1; i >= 0; i-- {
		if a.Focus.set(path[i].Widget, FocusChangePointer) {
			return
		}
	}
	_ = a.Focus.set(fallback, FocusChangePointer)
}

func (a *App) run(
	ctx context.Context,
	root Widget,
	out frameWriter,
	events <-chan Event,
	errs <-chan error,
	resizes <-chan term.Size,
	size Size,
) (runErr error) {
	prev := (*screen.Buffer)(nil)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	writeAll := func(data []byte) error {
		for len(data) > 0 {
			n, err := out.Write(data)
			if err != nil {
				return err
			}
			if n <= 0 {
				return io.ErrShortWrite
			}
			data = data[n:]
		}
		return nil
	}

	render := func() error {
		if !a.Dirty() {
			return nil
		}
		if a.takeForceRepaint() {
			// Discard the diff baseline so every cell and graphic is re-emitted.
			// Used after raw output (mpv video) has painted over the screen
			// outside our knowledge, so the widget tree fully repaints on return.
			prev = nil
		}
		next := a.Render(root, size)
		var frame []byte
		if a.ttyColors {
			if palette, ok := terminalANSI16Palette(out); ok {
				frame = screen.FrameWithPalette(prev, next, syncOutput(out), palette)
			} else {
				frame = screen.FrameWithColorMode(prev, next, syncOutput(out), screen.ColorModeTTY16)
			}
		} else {
			frame = screen.Frame(prev, next, syncOutput(out))
		}
		if len(frame) > 0 {
			if err := writeAll(frame); err != nil {
				return err
			}
		}
		prev = next
		return nil
	}
	flushRaw := func(limit int) error {
		for i := 0; i < limit; i++ {
			b := a.takeRaw()
			if len(b) == 0 {
				return nil
			}
			if err := writeAll(b); err != nil {
				return err
			}
		}
		return nil
	}
	// Close the root while the terminal writer and raw Kitty queue are still
	// alive. Shell.Close joins mpv's reader and queues its explicit final delete;
	// only then reject posts, execute already-accepted shutdown-aware closures,
	// and flush the finite raw backlog before term.Run closes the writer.
	defer func() {
		if closer, ok := root.(interface{ Close() }); ok {
			closer.Close()
		}
		a.stopPosts()
		a.drainPosts()
		if err := flushRaw(int(^uint(0) >> 1)); runErr == nil && err != nil {
			runErr = err
		}
		a.stopRawWrites()
	}()
	a.Invalidate()
	fastTick := false
	for {
		// Posts may stop mpv and request a force repaint. Flush every raw Kitty
		// command they queued before rendering, so mpv's final global delete can
		// never run after the restored frame.
		a.drainPosts()
		rawLimit := 1
		if a.forceRepaintPending() {
			// The video reader has stopped before requesting restoration, so this
			// backlog is finite and must be completely ordered before the repaint.
			rawLimit = int(^uint(0) >> 1)
		}
		if err := flushRaw(rawLimit); err != nil {
			return err
		}
		if err := render(); err != nil {
			// Canceling a prompt closes the terminal input to unblock its read.
			// A final render can race that close; cancellation is still a clean
			// shutdown, so do not surface the resulting write error to the user.
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return err
		}
		// Match the tick cadence to whether the tree is animating. Render just
		// ran, so a.animating() reflects the frame the user is about to see.
		if want := a.animating(); want != fastTick {
			fastTick = want
			if fastTick {
				ticker.Reset(animationTickInterval)
			} else {
				ticker.Reset(tickInterval)
			}
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		case <-a.wake:
		case <-ticker.C:
			if a.Handle(input.TickEvent{}) {
				a.Invalidate()
			}
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			if a.shouldExit(ev) {
				return nil
			}
			if a.Handle(ev) {
				a.Invalidate()
			}
		case err, ok := <-errs:
			if ok && err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			if !ok {
				errs = nil
			}
		case sz, ok := <-resizes:
			if ok {
				size = Size{W: sz.Width, H: sz.Height}
				a.Invalidate()
			} else {
				resizes = nil
			}
		}
	}
}

func (a *App) shouldExit(ev Event) bool {
	key, ok := ev.(input.KeyEvent)
	if !ok || key.Release {
		return false
	}
	if key.Key == input.KeyRune && key.Rune == 'c' && key.Mods&input.Ctrl != 0 {
		return true
	}
	if key.Key == input.KeyEsc {
		a.escExits--
		return a.escExits <= 0
	}
	a.escExits = 5
	return false
}

func (a *App) drainPosts() {
	for {
		a.mu.Lock()
		posts := a.posts
		a.posts = nil
		a.mu.Unlock()
		if len(posts) == 0 {
			return
		}
		for _, fn := range posts {
			fn()
		}
		a.mu.Lock()
		a.dirty = true
		a.mu.Unlock()
	}
}

func (a *App) signal() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

type frameWriter interface {
	Write([]byte) (int, error)
}

type syncCapable interface {
	Caps() term.Capabilities
}

func syncOutput(w frameWriter) bool {
	c, ok := w.(syncCapable)
	return ok && c.Caps().SyncOutput
}

func terminalANSI16Palette(w frameWriter) (screen.Palette, bool) {
	c, ok := w.(syncCapable)
	if !ok || !c.Caps().ANSI16Known {
		return screen.Palette{}, false
	}
	return c.Caps().ANSI16, true
}

func drawTree(buf *screen.Buffer, root Widget, hits HitIndex) {
	entries := hits.Entries()
	for _, entry := range entries {
		entry.Widget.Draw(buf.ClipWithin(screen.Rect{
			X: entry.Rect.X,
			Y: entry.Rect.Y,
			W: entry.Rect.W,
			H: entry.Rect.H,
		}, screen.Rect{
			X: entry.Clip.X,
			Y: entry.Clip.Y,
			W: entry.Clip.W,
			H: entry.Clip.H,
		}))
	}
	// Overlays (popups, toasts) draw on a higher layer so their painted cells
	// occlude graphics beneath them (see screen.Buffer.SetLayer): a menu or toast
	// over an inline image suppresses that image's Kitty placement instead of
	// letting it bleed through the overlay's background.
	buf.SetLayer(1)
	for _, entry := range entries {
		overlay, ok := entry.Widget.(Overlay)
		if !ok {
			continue
		}
		overlay.DrawOverlay(buf.ClipWithin(screen.Rect{
			X: entry.Rect.X,
			Y: entry.Rect.Y,
			W: entry.Rect.W,
			H: entry.Rect.H,
		}, screen.Rect{
			X: entry.Clip.X,
			Y: entry.Clip.Y,
			W: entry.Clip.W,
			H: entry.Clip.H,
		}))
	}
}

func measureHits(hits HitIndex) {
	for _, entry := range hits.Entries() {
		entry.Widget.Measure(Size{W: entry.Rect.W, H: entry.Rect.H})
	}
}

func collectWidgets(root Widget) []Widget {
	var widgets []Widget
	var walk func(Widget)
	walk = func(w Widget) {
		if w == nil {
			return
		}
		widgets = append(widgets, w)
		if container, ok := w.(Container); ok {
			for _, child := range container.Children() {
				walk(child)
			}
		}
	}
	walk(root)
	return widgets
}

func visibleWidgets(root Widget, hits HitIndex) []Widget {
	entries := hits.Entries()
	// A few lightweight embedders expose retained children without linking
	// their layout nodes. They cannot provide final visibility information, so
	// preserve the historical retained-tree fallback rather than dropping the
	// entire focus ring.
	if len(entries) == 0 {
		return collectWidgets(root)
	}
	widgets := make([]Widget, 0, len(entries))
	for _, entry := range entries {
		widgets = append(widgets, entry.Widget)
	}
	return widgets
}

func applyFocusIndicators(widgets []Widget, focused Widget) {
	for _, w := range widgets {
		if owner, ok := w.(FocusOwnerIndicator); ok {
			owner.SetFocusOwner(sameWidget(w, focused))
			continue
		}
		indicator, ok := w.(FocusIndicator)
		if !ok {
			continue
		}
		indicator.SetFocused(focused != nil && containsWidget(w, focused))
	}
}

func applyFocusPolicy(widgets []Widget, focusSplits bool) {
	for _, w := range widgets {
		if configurable, ok := w.(FocusConfigurable); ok {
			configurable.SetFocusEnabled(focusSplits)
		}
	}
}

func (a *App) focusChanged(change FocusChange) {
	if a == nil {
		return
	}
	a.mu.Lock()
	root := a.root
	a.mu.Unlock()
	if observer, ok := root.(FocusObserver); ok {
		observer.FocusChanged(change)
	}
}

func containsWidget(root, target Widget) bool {
	if sameWidget(root, target) {
		return true
	}
	container, ok := root.(Container)
	if !ok {
		return false
	}
	for _, child := range container.Children() {
		if containsWidget(child, target) {
			return true
		}
	}
	return false
}
