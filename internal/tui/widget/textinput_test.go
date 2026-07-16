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
