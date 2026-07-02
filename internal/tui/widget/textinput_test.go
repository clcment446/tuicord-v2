package widget

import (
	"testing"

	"awesomeProject/internal/tui/input"
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
