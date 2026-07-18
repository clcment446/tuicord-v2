package widget

import (
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
)

func TestTextInputGraphemeCursorAndBackspace(t *testing.T) {
	w := NewTextInput("")
	w.Handle(input.PasteEvent{Text: "a👩‍💻b"})

	w.Handle(input.KeyEvent{Key: input.KeyLeft})
	if got, want := w.Cursor(), len("a👩‍💻"); got != want {
		t.Fatalf("cursor after one left = %d, want %d", got, want)
	}

	w.Handle(input.KeyEvent{Key: input.KeyBackspace})
	if got, want := w.Value(), "ab"; got != want {
		t.Fatalf("value after backspace = %q, want %q", got, want)
	}
}

func TestTextInputPasteCombiningClusterDeletesAsUnit(t *testing.T) {
	w := NewTextInput("")
	w.Handle(input.PasteEvent{Text: "e\u0301!"})
	w.Handle(input.KeyEvent{Key: input.KeyLeft})
	w.Handle(input.KeyEvent{Key: input.KeyBackspace})

	if got, want := w.Value(), "!"; got != want {
		t.Fatalf("value after deleting combining cluster = %q, want %q", got, want)
	}
	if got := w.Cursor(); got != 0 {
		t.Fatalf("cursor after deleting combining cluster = %d, want 0", got)
	}
}

func TestTextInputReplaceReplacesGraphemeBoundedRange(t *testing.T) {
	w := NewTextInput("")
	w.SetValue("hi 👩‍💻 there")
	w.Replace(len("hi "), len("hi 👩‍💻"), "team")
	if got, want := w.Value(), "hi team there"; got != want {
		t.Fatalf("value = %q, want %q", got, want)
	}
	if got, want := w.Cursor(), len("hi team"); got != want {
		t.Fatalf("cursor = %d, want %d", got, want)
	}
}

func TestTextInputReplaceExpandsOffsetsInsideAGrapheme(t *testing.T) {
	w := NewTextInput("")
	w.SetValue("a👩‍💻b")
	w.Replace(2, 4, "x") // Both offsets fall within the emoji's UTF-8 bytes.
	if got, want := w.Value(), "axb"; got != want {
		t.Fatalf("value = %q, want %q", got, want)
	}
}

func TestTextInputCtrlNavigationAndBackspace(t *testing.T) {
	tests := []struct {
		name       string
		key        input.Key
		value      string
		cursor     int
		wantValue  string
		wantCursor int
	}{
		{
			name:       "ctrl left moves to previous word",
			key:        input.KeyLeft,
			value:      "hello world",
			cursor:     len("hello world"),
			wantValue:  "hello world",
			wantCursor: len("hello "),
		},
		{
			name:       "ctrl right moves to next word",
			key:        input.KeyRight,
			value:      "hello world",
			cursor:     0,
			wantValue:  "hello world",
			wantCursor: len("hello "),
		},
		{
			name:       "ctrl up moves to start",
			key:        input.KeyUp,
			value:      "hello world",
			cursor:     len("hello world"),
			wantValue:  "hello world",
			wantCursor: 0,
		},
		{
			name:       "ctrl down moves to end",
			key:        input.KeyDown,
			value:      "hello world",
			cursor:     0,
			wantValue:  "hello world",
			wantCursor: len("hello world"),
		},
		{
			name:       "ctrl backspace deletes preceding word",
			key:        input.KeyBackspace,
			value:      "hello world",
			cursor:     len("hello world"),
			wantValue:  "hello ",
			wantCursor: len("hello "),
		},
		{
			name:       "ctrl backspace deletes preceding word and whitespace",
			key:        input.KeyBackspace,
			value:      "hello world",
			cursor:     len("hello "),
			wantValue:  "world",
			wantCursor: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewTextInput("")
			w.SetValue(tt.value)
			if tt.cursor != len(tt.value) {
				w.SetCursor(tt.cursor)
			}

			if !w.Handle(input.KeyEvent{Key: tt.key, Mods: input.Ctrl}) {
				t.Fatal("Ctrl key was not handled")
			}
			if got := w.Value(); got != tt.wantValue {
				t.Fatalf("value = %q, want %q", got, tt.wantValue)
			}
			if got := w.Cursor(); got != tt.wantCursor {
				t.Fatalf("cursor = %d, want %d", got, tt.wantCursor)
			}
		})
	}
}

func TestTextInputCursorBlinksOnTick(t *testing.T) {
	w := NewTextInput("")
	w.SetValue("hi")

	buf := screen.NewBuffer(4, 1)
	w.Draw(buf.Clip(buf.Bounds()))
	if got := buf.Cell(2, 0).Style.Attrs & screen.Reverse; got == 0 {
		t.Fatal("cursor is not initially visible")
	}

	if !w.Handle(input.TickEvent{}) {
		t.Fatal("focused input did not consume tick")
	}
	buf = screen.NewBuffer(4, 1)
	w.Draw(buf.Clip(buf.Bounds()))
	if got := buf.Cell(2, 0).Style.Attrs & screen.Reverse; got != 0 {
		t.Fatal("cursor is still visible after blink tick")
	}

	w.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '!'})
	buf = screen.NewBuffer(4, 1)
	w.Draw(buf.Clip(buf.Bounds()))
	if got := buf.Cell(3, 0).Style.Attrs & screen.Reverse; got == 0 {
		t.Fatal("typing did not restore the cursor")
	}
}

func TestTextInputMultilineShiftEnterAndSubmit(t *testing.T) {
	w := NewTextInput("")
	w.SetMultiline(4)
	var submitted string
	w.OnSubmit(func(value string) { submitted = value })

	w.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'a'})
	w.Handle(input.KeyEvent{Key: input.KeyEnter, Mods: input.Shift})
	w.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'b'})
	if got, want := w.Value(), "a\nb"; got != want {
		t.Fatalf("value = %q, want %q", got, want)
	}
	w.Handle(input.KeyEvent{Key: input.KeyEnter})
	if got, want := submitted, "a\nb"; got != want {
		t.Fatalf("submitted = %q, want %q", got, want)
	}
}

func TestTextInputMultilineHandlesEncodedShiftEnter(t *testing.T) {
	w := NewTextInput("")
	w.SetMultiline(4)
	w.SetValue("a")
	submitted := false
	w.OnSubmit(func(string) { submitted = true })

	parser := input.NewParser()
	for _, event := range parser.Feed([]byte("\x1b[13;2u")) {
		w.Handle(event)
	}
	if submitted || w.Value() != "a\n" {
		t.Fatalf("encoded Shift+Enter produced value %q submitted=%v, want newline without submit", w.Value(), submitted)
	}
}

func TestTextInputFocusCanBeDisabledWithoutMakingItReadOnly(t *testing.T) {
	w := NewTextInput("Message")
	w.SetInputFocusEnabled(false)
	if w.CanFocus() {
		t.Fatal("normal-mode composer remained focusable")
	}
	if w.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '-'}) {
		t.Fatal("focus-disabled composer swallowed normal-mode dash")
	}
	if w.Value() != "" {
		t.Fatalf("focus-disabled composer value = %q", w.Value())
	}
	w.SetInputFocusEnabled(true)
	if !w.CanFocus() || !w.Handle(input.KeyEvent{Key: input.KeyRune, Rune: '-'}) || w.Value() != "-" {
		t.Fatalf("input-mode composer = focusable %v value %q", w.CanFocus(), w.Value())
	}
}

func TestTextInputMultilineWrapAndVerticalCursor(t *testing.T) {
	w := NewTextInput("")
	w.SetMultiline(3)
	w.SetValue("abcd")
	w.SetCursor(len("abc"))

	buf := screen.NewBuffer(2, 3)
	w.Draw(buf.Clip(buf.Bounds()))
	if got := buf.Cell(0, 1).Content; got != "c" {
		t.Fatalf("wrapped row begins with %q, want c", got)
	}
	w.Handle(input.KeyEvent{Key: input.KeyUp})
	if got, want := w.Cursor(), 1; got != want {
		t.Fatalf("cursor after up = %d, want %d", got, want)
	}
	w.Handle(input.KeyEvent{Key: input.KeyDown})
	if got, want := w.Cursor(), 3; got != want {
		t.Fatalf("cursor after down = %d, want %d", got, want)
	}
}

func TestTextInputDeclinesCtrlModifiedRunes(t *testing.T) {
	w := NewTextInput("")
	// ctrl+v is a shortcut, not text: the input must decline it so it bubbles
	// to global key handling (e.g. paste-image), not insert "v".
	if w.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'v', Mods: input.Ctrl}) {
		t.Fatal("text input handled ctrl+v instead of declining it")
	}
	if w.Value() != "" {
		t.Fatalf("ctrl+v inserted %q, want empty", w.Value())
	}
	// A plain rune still inserts.
	if !w.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'v'}) {
		t.Fatal("plain rune was not inserted")
	}
	if w.Value() != "v" {
		t.Fatalf("value = %q, want \"v\"", w.Value())
	}
}
