package tui

import (
	"context"
	"errors"
	"io"
	"sync"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/term"
)

// Option configures an App.
type Option func(*App)

// App is the runtime coordinator for a retained widget tree.
type App struct {
	mu    sync.Mutex
	root  Widget
	size  Size
	hits  HitIndex
	dirty bool
	posts []func()
	wake  chan struct{}
	theme Theme

	// Focus owns keyboard focus traversal for the retained tree.
	Focus FocusManager
	// Drag owns pointer capture for draggable widgets.
	Drag DragManager
}

// New returns an App with default runtime state.
func New(opts ...Option) *App {
	a := &App{
		dirty: true,
		wake:  make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
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
	drawTree(buf, root, hits)
	widgets := collectWidgets(root)

	a.mu.Lock()
	a.root = root
	a.size = size
	a.hits = hits
	a.dirty = false
	a.mu.Unlock()
	a.Focus.Replace(widgets)
	return buf
}

// Handle routes an input event through drag, focus, and mouse hit testing.
func (a *App) Handle(ev Event) bool {
	if a == nil || ev == nil {
		return false
	}
	switch ev := ev.(type) {
	case input.MouseEvent:
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
	return term.Run(func(t *term.Terminal) error {
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
		if path[i].Widget.Handle(ev) {
			return true
		}
	}
	return false
}

func (a *App) handleKey(ev input.KeyEvent) bool {
	if a.Drag.Active() && ev.Key == input.KeyEsc && !ev.Release {
		a.Drag.Cancel()
		a.Invalidate()
		return true
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
	return a.handleFocused(ev)
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
	render := func() error {
		a.drainPosts()
		if !a.Dirty() {
			return nil
		}
		next := a.Render(root, size)
		frame := screen.Frame(prev, next, syncOutput(out))
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
			return err
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		case <-a.wake:
		case ev, ok := <-events:
			if !ok {
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
