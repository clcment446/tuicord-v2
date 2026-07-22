package widget

import (
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

// press builds a left-button mouse press at (x, y).
func press(x, y int) input.MouseEvent {
	return input.MouseEvent{X: x, Y: y, Btn: input.ButtonLeft, Kind: input.MousePress}
}

func key(k input.Key) input.KeyEvent { return input.KeyEvent{Key: k} }

func TestMenuInitialSelectionSkipsNonSelectable(t *testing.T) {
	m := NewMenu([]MenuItem{
		{Separator: true},
		{Label: "disabled", Disabled: true},
		{Label: "reply"},
		{Label: "delete"},
	})
	if got := m.Selected(); got != 2 {
		t.Fatalf("initial selection = %d, want 2 (first selectable)", got)
	}
}

func TestMenuArrowNavigationSkipsSeparatorsAndDisabled(t *testing.T) {
	m := NewMenu([]MenuItem{
		{Label: "reply"},
		{Separator: true},
		{Label: "edit", Disabled: true},
		{Label: "delete"},
	})
	// Down from "reply" (0) should land on "delete" (3), skipping 1 and 2.
	m.Handle(key(input.KeyDown))
	if got := m.Selected(); got != 3 {
		t.Fatalf("after Down selection = %d, want 3", got)
	}
	// Down again clamps (no wrap).
	m.Handle(key(input.KeyDown))
	if got := m.Selected(); got != 3 {
		t.Fatalf("after second Down selection = %d, want 3 (no wrap)", got)
	}
	// Up returns to "reply".
	m.Handle(key(input.KeyUp))
	if got := m.Selected(); got != 0 {
		t.Fatalf("after Up selection = %d, want 0", got)
	}
}

func TestMenuVimJKNavigation(t *testing.T) {
	m := NewMenu([]MenuItem{{Label: "reply"}, {Label: "delete"}})
	m.SetVimNavigation(true)
	m.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'j'})
	if got := m.Selected(); got != 1 {
		t.Fatalf("after j selection = %d, want 1", got)
	}
	m.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'k'})
	if got := m.Selected(); got != 0 {
		t.Fatalf("after k selection = %d, want 0", got)
	}
}

func TestMenuEnterActivatesAndDismisses(t *testing.T) {
	var chosen string
	dismissed := false
	m := NewMenu([]MenuItem{
		{Label: "reply", OnSelect: func() { chosen = "reply" }},
		{Label: "delete", OnSelect: func() { chosen = "delete" }},
	})
	m.OnDismiss(func() { dismissed = true })

	m.Handle(key(input.KeyDown)) // select "delete"
	if !m.Handle(key(input.KeyEnter)) {
		t.Fatal("Enter not handled")
	}
	if chosen != "delete" {
		t.Fatalf("chosen = %q, want delete", chosen)
	}
	if !dismissed {
		t.Fatal("activating an item should dismiss the menu")
	}
}

func TestMenuEscDismissesWithoutSelecting(t *testing.T) {
	activated := false
	dismissed := false
	m := NewMenu([]MenuItem{{Label: "reply", OnSelect: func() { activated = true }}})
	m.OnDismiss(func() { dismissed = true })

	if !m.Handle(key(input.KeyEsc)) {
		t.Fatal("Esc not handled")
	}
	if activated {
		t.Fatal("Esc must not activate an item")
	}
	if !dismissed {
		t.Fatal("Esc must dismiss")
	}
}

func TestMenuKeysAreModalSwallowed(t *testing.T) {
	m := NewMenu([]MenuItem{{Label: "reply"}})
	// An unrelated printable key must be consumed, not leaked to the layer behind.
	if !m.Handle(input.KeyEvent{Key: input.KeyRune, Rune: 'x'}) {
		t.Fatal("modal menu must swallow stray keys")
	}
	// Key releases pass through.
	if m.Handle(input.KeyEvent{Key: input.KeyEnter, Release: true}) {
		t.Fatal("key release should not be consumed")
	}
}

func TestMenuClickInsideActivatesRow(t *testing.T) {
	var chosen string
	m := NewMenu([]MenuItem{
		{Label: "reply", OnSelect: func() { chosen = "reply" }},
		{Label: "delete", OnSelect: func() { chosen = "delete" }},
	})
	m.OnDismiss(func() {})
	m.SetAnchor(0, 0)
	// Render once so the box rect is known.
	buf := screen.NewBuffer(20, 8)
	m.Measure(sizeOf(buf))
	m.Draw(buf.Clip(buf.Bounds()))

	// Row 1 ("delete") sits at y = boxY(0) + 1 + index(1) = 2, inside the border.
	if !m.Handle(press(3, 2)) {
		t.Fatal("click inside not handled")
	}
	if chosen != "delete" {
		t.Fatalf("chosen = %q, want delete", chosen)
	}
}

func TestMenuClickOnClippedRowDoesNotActivateInvisibleEntry(t *testing.T) {
	activated := -1
	items := make([]MenuItem, 10)
	for i := range items {
		idx := i
		items[i] = MenuItem{Label: "item", OnSelect: func() { activated = idx }}
	}
	m := NewMenu(items)
	m.OnDismiss(func() {})
	m.SetAnchor(0, 0)
	// A short screen clips the menu: with height 5 the box is border, three inner
	// rows, border. Items past index 2 are never drawn.
	buf := screen.NewBuffer(20, 5)
	m.Measure(sizeOf(buf))
	m.Draw(buf.Clip(buf.Bounds()))

	rect := m.last
	// Clicking the bottom border row must not activate the (undrawn) item that its
	// naive row index would map to.
	m.Handle(press(rect.X+1, rect.Y+rect.H-1))
	if activated != -1 {
		t.Fatalf("clicking the clipped bottom border activated invisible item %d", activated)
	}
	// A visible inner row still activates normally.
	m.Handle(press(rect.X+1, rect.Y+1))
	if activated != 0 {
		t.Fatalf("visible row activation = %d, want 0", activated)
	}
}

func TestMenuClickOutsideDismisses(t *testing.T) {
	dismissed := false
	activated := false
	m := NewMenu([]MenuItem{{Label: "reply", OnSelect: func() { activated = true }}})
	m.OnDismiss(func() { dismissed = true })
	m.SetAnchor(0, 0)
	buf := screen.NewBuffer(20, 8)
	m.Measure(sizeOf(buf))
	m.Draw(buf.Clip(buf.Bounds()))

	// Far corner is outside the small box.
	if !m.Handle(press(19, 7)) {
		t.Fatal("click outside not handled")
	}
	if activated {
		t.Fatal("click outside must not activate")
	}
	if !dismissed {
		t.Fatal("click outside must dismiss")
	}
}

func TestMenuClickOnDisabledRowDoesNothing(t *testing.T) {
	activated := false
	dismissed := false
	m := NewMenu([]MenuItem{
		{Label: "edit", Disabled: true, OnSelect: func() { activated = true }},
		{Label: "reply"},
	})
	m.OnDismiss(func() { dismissed = true })
	m.SetAnchor(0, 0)
	buf := screen.NewBuffer(20, 8)
	m.Measure(sizeOf(buf))
	m.Draw(buf.Clip(buf.Bounds()))

	// Disabled "edit" is row 0 → y = 1.
	m.Handle(press(3, 1))
	if activated {
		t.Fatal("disabled row must not activate")
	}
	if dismissed {
		t.Fatal("clicking a disabled row inside the box must not dismiss")
	}
}

func TestMenuClampsToBottomRightCorner(t *testing.T) {
	m := NewMenu([]MenuItem{{Label: "reply"}, {Label: "delete"}})
	m.SetAnchor(100, 100) // way past the screen
	buf := screen.NewBuffer(20, 8)
	m.Measure(sizeOf(buf))
	m.Draw(buf.Clip(buf.Bounds()))

	rect := m.last
	if rect.X+rect.W > 20 || rect.Y+rect.H > 8 {
		t.Fatalf("box %+v exceeds 20x8 screen", rect)
	}
	if rect.X < 0 || rect.Y < 0 {
		t.Fatalf("box %+v has negative origin", rect)
	}
}

func TestMenuAllDisabledHasNoSelection(t *testing.T) {
	m := NewMenu([]MenuItem{
		{Separator: true},
		{Label: "a", Disabled: true},
	})
	if got := m.Selected(); got != -1 {
		t.Fatalf("selection = %d, want -1 when nothing is selectable", got)
	}
	// Navigation on an unselectable menu must not panic or select.
	m.Handle(key(input.KeyDown))
	if got := m.Selected(); got != -1 {
		t.Fatalf("selection after Down = %d, want -1", got)
	}
}

func sizeOf(b *screen.Buffer) tui.Size {
	r := b.Bounds()
	return tui.Size{W: r.W, H: r.H}
}
