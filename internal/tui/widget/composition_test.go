package widget

import (
	"image"
	"image/color"
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/tui"
)

func TestViewportCanHostButtonColumn(t *testing.T) {
	first := NewButton("one", nil)
	second := NewButton("two", nil)
	list := Column(first, second)
	viewport := NewViewport()
	viewport.SetChild(list)
	viewport.SetScroll(0, 1)

	buf := tui.New().Render(viewport, tui.Size{W: 3, H: 1})
	if got, want := bufferRow(buf, 0), "two"; got != want {
		t.Fatalf("row = %q, want %q", got, want)
	}
}

func TestFocusedPathFallbackDoesNotBroadcastIntoUnfocusedInput(t *testing.T) {
	button := NewButton("nav", nil)
	composer := NewTextInput("message")
	root := Column(button, composer)
	app := tui.New()
	app.Render(root, tui.Size{W: 20, H: 2})
	if !app.Focus.Set(button) {
		t.Fatal("could not focus navigation button")
	}

	if app.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'x'}) {
		t.Fatal("ordinary key was unexpectedly consumed")
	}
	if app.Handle(input.PasteEvent{Text: "hidden paste"}) {
		t.Fatal("ordinary paste was unexpectedly consumed")
	}
	if composer.Value() != "" {
		t.Fatalf("unfocused composer received fallback input: %q", composer.Value())
	}
}

func TestDraggableWrapsAnyWidget(t *testing.T) {
	drag := NewDraggable(NewText("x"))
	drag.SetPosition(1, 0)
	buf := tui.New().Render(drag, tui.Size{W: 3, H: 1})
	if got := buf.Cell(1, 0).Content; got != "x" {
		t.Fatalf("dragged child content = %q, want x", got)
	}
}

func TestViewportCanHostImageWidget(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.Black)
	img.Set(1, 0, color.White)
	picture := NewImageFrom(img)
	picture.SetMode(ImageASCII)
	viewport := NewViewport()
	viewport.SetChild(picture)

	buf := tui.New().Render(viewport, tui.Size{W: 2, H: 1})
	if got, want := bufferRow(buf, 0), " @"; got != want {
		t.Fatalf("viewport image row = %q, want %q", got, want)
	}
}
