package tui

import (
	"context"
	"errors"
	"io"
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
)

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

func (w *overlayWidget) DrawOverlay(r screen.Region) {
	r.Set(0, 0, screen.Cell{Content: w.overlay})
}
