package tui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
)

func TestWriteRawIsNonBlockingBoundedAndAtomic(t *testing.T) {
	app := New()
	for i := 0; i < 100; i++ {
		app.WriteRaw([]byte{byte(i), byte(i + 1)})
	}
	if got := len(app.rawWrites); got != 8 {
		t.Fatalf("queued raw writes = %d, want 8", got)
	}
	for _, command := range app.rawWrites {
		if len(command) != 2 {
			t.Fatalf("fragmented command = %v", command)
		}
	}
	input := []byte("complete kitty command")
	app.WriteRaw(input)
	input[0] = 'X'
	if got := app.rawWrites[len(app.rawWrites)-1][0]; got != 'c' {
		t.Fatalf("queued command aliases caller buffer: %q", got)
	}
}

func TestWriteRawIgnoresWritesAfterShutdown(t *testing.T) {
	app := New()
	app.WriteRaw([]byte("before"))
	app.stopRawWrites()
	app.WriteRaw([]byte("after"))
	if len(app.rawWrites) != 0 {
		t.Fatalf("raw writes retained after shutdown: %q", app.rawWrites)
	}
}

func TestTryPostRejectsAfterShutdown(t *testing.T) {
	app := New()
	app.stopPosts()
	if app.TryPost(func() {}) {
		t.Fatal("TryPost accepted work after shutdown")
	}
	if len(app.posts) != 0 {
		t.Fatalf("post retained after shutdown: %d", len(app.posts))
	}
}

func TestPostRunsInFIFOOrder(t *testing.T) {
	app := New()
	var got []int
	app.Post(func() { got = append(got, 1) })
	app.Post(func() { got = append(got, 2) })
	app.Post(func() {
		got = append(got, 3)
		app.Post(func() { got = append(got, 4) })
	})

	app.drainPosts()
	want := []int{1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("posts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("posts = %v, want %v", got, want)
		}
	}
	if !app.Dirty() {
		t.Fatal("Post drain did not invalidate")
	}
}

func TestRenderDrawsParentBeforeChildren(t *testing.T) {
	child := &drawWidget{
		testWidget: *newTestWidget("child", false),
		content:    "c",
	}
	child.node.Grow = 1
	parent := &drawWidget{
		testWidget: *newTestWidget("parent", false),
		content:    "p",
	}
	parent.node = &layout.Node{Children: []*layout.Node{child.node}}
	parent.children = []Widget{child}

	buf := New().Render(parent, Size{W: 1, H: 1})
	if got := buf.Cell(0, 0).Content; got != "c" {
		t.Fatalf("rendered cell = %q, want child drawn over parent", got)
	}
}

func TestCtrlCExitsBeforeWidgetHandling(t *testing.T) {
	app := New()
	events := make(chan Event, 1)
	events <- input.KeyEvent{Key: input.KeyRune, Rune: 'c', Mods: input.Ctrl}
	root := &handlingWidget{testWidget: *newTestWidget("root", false)}

	if err := app.run(context.Background(), root, discardWriter{}, events, nil, nil, Size{W: 1, H: 1}); err != nil {
		t.Fatal(err)
	}
	if root.handled != 0 {
		t.Fatalf("root handled %d events, want 0", root.handled)
	}
}

func TestShutdownClosesRootAndFlushesFinalRawWrite(t *testing.T) {
	tests := []struct {
		name   string
		ctx    func() context.Context
		events []Event
	}{
		{name: "ctrl-c", ctx: context.Background, events: []Event{input.KeyEvent{Key: input.KeyRune, Rune: 'c', Mods: input.Ctrl}}},
		{name: "five-esc", ctx: context.Background, events: []Event{
			input.KeyEvent{Key: input.KeyEsc}, input.KeyEvent{Key: input.KeyEsc}, input.KeyEvent{Key: input.KeyEsc}, input.KeyEvent{Key: input.KeyEsc}, input.KeyEvent{Key: input.KeyEsc},
		}},
		{name: "signal-context", ctx: func() context.Context {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := New()
			var events chan Event
			if len(tt.events) > 0 {
				events = make(chan Event, len(tt.events))
				for _, ev := range tt.events {
					events <- ev
				}
			}
			root := &closingRoot{handlingWidget: handlingWidget{testWidget: *newTestWidget("root", false)}, app: app}
			var out bytes.Buffer

			if err := app.run(tt.ctx(), root, &out, events, nil, nil, Size{W: 1, H: 1}); err != nil {
				t.Fatal(err)
			}
			if !root.closed {
				t.Fatal("root was not closed before event-loop shutdown")
			}
			if !bytes.HasSuffix(out.Bytes(), []byte("FINAL_KITTY_DELETE")) {
				t.Fatalf("final raw delete was not flushed last: %q", out.Bytes())
			}
		})
	}
}

func TestFiveEscapesExit(t *testing.T) {
	app := New()
	events := make(chan Event, 5)
	for i := 0; i < 5; i++ {
		events <- input.KeyEvent{Key: input.KeyEsc}
	}
	root := &handlingWidget{testWidget: *newTestWidget("root", false)}

	if err := app.run(context.Background(), root, discardWriter{}, events, nil, nil, Size{W: 1, H: 1}); err != nil {
		t.Fatal(err)
	}
	if root.handled != 4 {
		t.Fatalf("root handled %d events, want first 4 Esc events before exit", root.handled)
	}
}

func TestNonEscapeResetsEscapeExitCount(t *testing.T) {
	app := New()
	for i := 0; i < 4; i++ {
		if app.shouldExit(input.KeyEvent{Key: input.KeyEsc}) {
			t.Fatalf("shouldExit returned true at Esc %d", i+1)
		}
	}
	if app.shouldExit(input.KeyEvent{Key: input.KeyRune, Rune: 'x'}) {
		t.Fatal("non-Escape key exited")
	}
	for i := 0; i < 4; i++ {
		if app.shouldExit(input.KeyEvent{Key: input.KeyEsc}) {
			t.Fatalf("escape count did not reset; exited at Esc %d after reset", i+1)
		}
	}
	if !app.shouldExit(input.KeyEvent{Key: input.KeyEsc}) {
		t.Fatal("fifth Esc after reset did not exit")
	}
}

func TestMouseEventsAreIgnoredWhenMouseModeIsOff(t *testing.T) {
	app := New(WithMouse(false))
	widget := &handlingWidget{testWidget: *newTestWidget("mouse", true)}
	app.Render(widget, Size{W: 5, H: 1})

	if app.Handle(input.MouseEvent{X: 1, Y: 0, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("mouse event should be ignored when mouse mode is off")
	}
	if widget.handled != 0 {
		t.Fatalf("widget handled %d mouse events, want 0", widget.handled)
	}
}

func TestFocusableSplitsCanBeEnabledByAppOption(t *testing.T) {
	app := New(WithFocusableSplits(true))
	first := &handlingWidget{testWidget: *newTestWidget("first", true)}
	second := &handlingWidget{testWidget: *newTestWidget("second", true)}
	root := &splitLikeWidget{children: []Widget{first, second}}
	app.Render(root, Size{W: 10, H: 1})

	if got := app.Focus.Len(); got != 3 {
		t.Fatalf("focus ring length = %d, want split and two children", got)
	}
	if got := app.Focus.Focused(); got != root {
		t.Fatal("focusable split should receive initial focus")
	}
}

func TestAltArrowsNavigateFocusHistory(t *testing.T) {
	first := &handlingWidget{testWidget: *newTestWidget("first", true)}
	second := &handlingWidget{testWidget: *newTestWidget("second", true)}
	third := &handlingWidget{testWidget: *newTestWidget("third", true)}
	root := &splitLikeWidget{children: []Widget{first, second, third}}
	app := New()
	app.Render(root, Size{W: 10, H: 1})
	app.Focus.Set(second)
	app.Focus.Set(third)

	if !app.Handle(input.KeyEvent{Key: input.KeyLeft, Mods: input.Alt}) {
		t.Fatal("Alt+Left was not handled")
	}
	if got := app.Focus.Focused(); got != second {
		t.Fatalf("Alt+Left focused %v, want second", got)
	}
	if !app.Handle(input.KeyEvent{Key: input.KeyRight, Mods: input.Alt}) {
		t.Fatal("Alt+Right was not handled")
	}
	if got := app.Focus.Focused(); got != third {
		t.Fatalf("Alt+Right focused %v, want third", got)
	}
}

func TestVimHLTraverseFocusAndAllowLocalUnfold(t *testing.T) {
	first := &vimFocusWidget{handlingWidget: handlingWidget{testWidget: *newTestWidget("first", true)}, enabled: true}
	second := &handlingWidget{testWidget: *newTestWidget("second", true)}
	root := &splitLikeWidget{children: []Widget{first, second}}
	app := New()
	app.Render(root, Size{W: 10, H: 1})

	if !app.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'l'}) || app.Focus.Focused() != second {
		t.Fatal("l did not traverse forward like Tab")
	}
	app.Focus.Set(first)
	first.consume = true
	if !app.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'h'}) || app.Focus.Focused() != first {
		t.Fatal("local unfold did not keep focus on the Vim-aware widget")
	}
	if first.calls != 2 {
		t.Fatalf("Vim focus calls = %d, want 2", first.calls)
	}
}

func TestVimUppercaseHLTraverseSections(t *testing.T) {
	first := &vimFocusWidget{handlingWidget: handlingWidget{testWidget: *newTestWidget("first", true)}, enabled: true}
	second := &vimFocusWidget{handlingWidget: handlingWidget{testWidget: *newTestWidget("second", true)}, enabled: true}
	app := New()
	app.Render(&splitLikeWidget{children: []Widget{first, second}}, Size{W: 10, H: 1})

	if !app.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'L'}) || app.Focus.Focused() != second {
		t.Fatal("L did not switch to the next section")
	}
	if !app.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'H'}) || app.Focus.Focused() != first {
		t.Fatal("H did not switch to the previous section")
	}
	if first.calls != 0 {
		t.Fatalf("uppercase section navigation called local component traversal %d times", first.calls)
	}
}

func TestVimHLDoesNotTraverseWhenWidgetHasNotOptedIn(t *testing.T) {
	first := &vimFocusWidget{handlingWidget: handlingWidget{testWidget: *newTestWidget("first", true)}}
	second := &handlingWidget{testWidget: *newTestWidget("second", true)}
	app := New()
	app.Render(&splitLikeWidget{children: []Widget{first, second}}, Size{W: 10, H: 1})
	app.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'l'})
	if app.Focus.Focused() != first {
		t.Fatal("focus moved while Vim navigation was disabled")
	}
	if first.calls != 0 {
		t.Fatal("disabled Vim traverser was invoked")
	}
}

func TestRootFocusRequestMovesFocusAfterHandledKey(t *testing.T) {
	first := newTestWidget("first", true)
	second := newTestWidget("second", true)
	root := &focusRequestRoot{splitLikeWidget: splitLikeWidget{children: []Widget{first, second}}, target: second}
	app := New()
	app.Render(root, Size{W: 10, H: 1})
	if !app.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'I'}) {
		t.Fatal("input-mode key was not handled")
	}
	if app.Focus.Focused() != second {
		t.Fatalf("focused = %v, want requested second widget", app.Focus.Focused())
	}
}

func TestOverlayDrawsAfterChildren(t *testing.T) {
	child := &drawWidget{
		testWidget: *newTestWidget("child", false),
		content:    "c",
	}
	child.node.Grow = 1
	parent := &overlayWidget{
		drawWidget: drawWidget{
			testWidget: *newTestWidget("parent", false),
			content:    "p",
		},
		overlay: "o",
	}
	parent.node = &layout.Node{Children: []*layout.Node{child.node}}
	parent.children = []Widget{child}

	buf := New().Render(parent, Size{W: 1, H: 1})
	if got := buf.Cell(0, 0).Content; got != "o" {
		t.Fatalf("rendered cell = %q, want overlay drawn above child", got)
	}
}

func TestEventOverlayPreemptsFocusedChild(t *testing.T) {
	child := &handlingWidget{testWidget: *newTestWidget("child", true)}
	root := &eventOverlayWidget{
		overlayWidget: overlayWidget{drawWidget: drawWidget{testWidget: *newTestWidget("root", false)}},
	}
	root.node = &layout.Node{Children: []*layout.Node{child.node}}
	root.children = []Widget{child}
	app := New()
	app.Render(root, Size{W: 10, H: 1})
	if !app.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'a'}) {
		t.Fatal("event overlay did not consume key")
	}
	if child.handled != 0 || root.events != 1 {
		t.Fatalf("child events = %d, overlay events = %d; want 0, 1", child.handled, root.events)
	}
}

func TestCanceledContextSwallowsWriteError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := &drawWidget{testWidget: *newTestWidget("root", false), content: "x"}
	root.node.Grow = 1

	err := New().run(ctx, root, errWriter{}, nil, nil, nil, Size{W: 1, H: 1})
	if err != nil {
		t.Fatalf("run returned %v, want nil after canceled shutdown", err)
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return io.Discard.Write(p) }

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("file already closed") }

type handlingWidget struct {
	testWidget
	handled int
}

type closingRoot struct {
	handlingWidget
	app    *App
	closed bool
}

func (r *closingRoot) Close() {
	r.closed = true
	r.app.WriteRaw([]byte("FINAL_KITTY_DELETE"))
}

type vimFocusWidget struct {
	handlingWidget
	consume bool
	enabled bool
	calls   int
}

func (w *vimFocusWidget) VimFocusEnabled() bool { return w.enabled }

func (w *vimFocusWidget) HandleVimFocus(bool) bool {
	w.calls++
	return w.consume
}

type focusRequestRoot struct {
	splitLikeWidget
	target Widget
	ready  bool
}

func (w *focusRequestRoot) Handle(ev Event) bool {
	key, ok := ev.(input.KeyEvent)
	if ok && key.Key == input.KeyRune && key.Rune == 'I' {
		w.ready = true
		return true
	}
	return false
}

func (w *focusRequestRoot) TakeFocusRequest() Widget {
	if !w.ready {
		return nil
	}
	w.ready = false
	return w.target
}

func (w *handlingWidget) Handle(Event) bool {
	w.handled++
	return true
}

type splitLikeWidget struct {
	testWidget
	children []Widget
	focused  bool
}

func (w *splitLikeWidget) SetFocusEnabled(enabled bool) { w.focus = enabled }
func (w *splitLikeWidget) CanFocus() bool               { return w.focus }
func (w *splitLikeWidget) Children() []Widget           { return w.children }

type drawWidget struct {
	testWidget
	content string
}

func (w *drawWidget) Draw(r screen.Region) {
	r.Set(0, 0, screen.Cell{Content: w.content})
}

type overlayWidget struct {
	drawWidget
	overlay string
}

type eventOverlayWidget struct {
	overlayWidget
	children []Widget
	events   int
}

func (w *eventOverlayWidget) Children() []Widget { return w.children }
func (w *eventOverlayWidget) HandleOverlay(Event) bool {
	w.events++
	return true
}

func (w *overlayWidget) DrawOverlay(r screen.Region) {
	r.Set(0, 0, screen.Cell{Content: w.overlay})
}
