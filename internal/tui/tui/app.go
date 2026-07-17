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

const tickInterval = 500 * time.Millisecond

// Option configures an App.
type Option func(*App)

// App is the runtime coordinator for a retained widget tree.
type App struct {
	mu          sync.Mutex
	root        Widget
	size        Size
	hits        HitIndex
	dirty       bool
	posts       []func()
	wake        chan struct{}
	theme       Theme
	escExits    int
	mouseOn     bool
	focusSplits bool
	ttyColors   bool

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
	return a
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
func (a *App) Post(fn func()) {
	if a == nil || fn == nil {
		return
	}
	a.mu.Lock()
	a.posts = append(a.posts, fn)
	a.mu.Unlock()
	a.signal()
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

// Dirty reports whether the app currently needs to redraw.
func (a *App) Dirty() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dirty
}

// Render draws root at size and refreshes the hit-test and focus indexes.
func (a *App) Render(root Widget, size Size) *screen.Buffer {
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
	if root == nil || size.W == 0 || size.H == 0 {
		a.mu.Lock()
		a.root = root
		a.size = size
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
	a.Focus.Replace(widgets)
	applyFocusIndicators(widgets, a.Focus.Focused())
	drawTree(buf, root, hits)

	a.mu.Lock()
	a.root = root
	a.size = size
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
		return a.handleMouse(ev)
	case input.KeyEvent:
		return a.handleKey(ev)
	default:
		return a.handleFocused(ev)
	}
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
	if ev.Key == input.KeyTab && !ev.Release {
		if ev.Mods&input.Shift != 0 {
			a.Focus.Prev()
		} else {
			a.Focus.Next()
		}
		a.Invalidate()
		return true
	}
	handled := a.handleFocused(ev)
	if requester, ok := root.(FocusRequester); ok {
		if requested := requester.TakeFocusRequest(); requested != nil {
			a.Focus.Set(requested)
			a.Invalidate()
		}
	}
	return handled
}

func (a *App) handleFocused(ev Event) bool {
	if focused := a.Focus.Focused(); focused != nil && focused.Handle(ev) {
		return true
	}
	a.mu.Lock()
	root := a.root
	a.mu.Unlock()
	if root != nil && !sameWidget(root, a.Focus.Focused()) {
		return root.Handle(ev)
	}
	return false
}

func (a *App) focusDeepest(path []Hit, fallback Widget) {
	for i := len(path) - 1; i >= 0; i-- {
		if a.Focus.Set(path[i].Widget) {
			return
		}
	}
	_ = a.Focus.Set(fallback)
}

func (a *App) run(
	ctx context.Context,
	root Widget,
	out frameWriter,
	events <-chan Event,
	errs <-chan error,
	resizes <-chan term.Size,
	size Size,
) error {
	prev := (*screen.Buffer)(nil)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	render := func() error {
		a.drainPosts()
		if !a.Dirty() {
			return nil
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
			if _, err := out.Write(frame); err != nil {
				return err
			}
		}
		prev = next
		return nil
	}
	a.Invalidate()
	for {
		if err := render(); err != nil {
			// Canceling a prompt closes the terminal input to unblock its read.
			// A final render can race that close; cancellation is still a clean
			// shutdown, so do not surface the resulting write error to the user.
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return err
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
