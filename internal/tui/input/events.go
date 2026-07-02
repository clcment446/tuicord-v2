// Package input turns terminal bytes into typed input events.
//
// Parser is pure: feed it bytes and drain events, with no IO or goroutines.
// Reader is the thin shell that owns an input stream and resolves the legacy
// ESC-vs-Alt ambiguity with a timeout. Pasted text is delivered atomically so
// keybindings never fire for paste content.
package input

// Event is any decoded terminal input event.
type Event interface {
	inputEvent()
}

// Mod is a bitset of keyboard or pointer modifiers.
type Mod uint8

const (
	// Shift is the shift modifier.
	Shift Mod = 1 << iota
	// Alt is the alt/option modifier.
	Alt
	// Ctrl is the control modifier.
	Ctrl
	// Super is the super/command/windows modifier when reported by Kitty.
	Super
	// Hyper is the hyper modifier when reported by Kitty.
	Hyper
	// Meta is the meta modifier when reported by Kitty.
	Meta
)

// Key identifies non-text keys. Printable input uses KeyRune with Rune set.
type Key int

const (
	// KeyUnknown is used only when a known protocol does not identify a key.
	KeyUnknown Key = iota
	// KeyRune is a printable Unicode code point.
	KeyRune
	// KeyEsc is Escape.
	KeyEsc
	// KeyEnter is Enter or Return.
	KeyEnter
	// KeyTab is Tab.
	KeyTab
	// KeyBackspace is Backspace.
	KeyBackspace
	// KeyUp is the up arrow.
	KeyUp
	// KeyDown is the down arrow.
	KeyDown
	// KeyRight is the right arrow.
	KeyRight
	// KeyLeft is the left arrow.
	KeyLeft
	// KeyHome is Home.
	KeyHome
	// KeyEnd is End.
	KeyEnd
	// KeyPageUp is Page Up.
	KeyPageUp
	// KeyPageDown is Page Down.
	KeyPageDown
	// KeyDelete is Delete.
	KeyDelete
	// KeyInsert is Insert.
	KeyInsert
	// KeyF1 through KeyF12 are function keys.
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
)

// KeyEvent is a keyboard event. Rune is set only when Key is KeyRune.
type KeyEvent struct {
	Key     Key
	Mods    Mod
	Rune    rune
	Release bool
}

// Button identifies a mouse button or wheel direction.
type Button int

const (
	// ButtonNone is used for releases that do not carry a specific button.
	ButtonNone Button = iota
	// ButtonLeft is the primary mouse button.
	ButtonLeft
	// ButtonMiddle is the middle mouse button.
	ButtonMiddle
	// ButtonRight is the secondary mouse button.
	ButtonRight
	// ButtonWheelUp is a wheel-up tick.
	ButtonWheelUp
	// ButtonWheelDown is a wheel-down tick.
	ButtonWheelDown
	// ButtonWheelLeft is a horizontal wheel-left tick.
	ButtonWheelLeft
	// ButtonWheelRight is a horizontal wheel-right tick.
	ButtonWheelRight
)

// MouseKind describes the pointer action.
type MouseKind int

const (
	// MousePress is a button press.
	MousePress MouseKind = iota
	// MouseRelease is a button release.
	MouseRelease
	// MouseMotion is movement with a pressed button or motion tracking.
	MouseMotion
	// MouseWheel is a wheel tick.
	MouseWheel
)

// MouseEvent is an SGR-1006 mouse event. X and Y are zero-based cell
// coordinates.
type MouseEvent struct {
	X, Y int
	Btn  Button
	Mods Mod
	Kind MouseKind
}

// PasteEvent is a complete bracketed-paste payload.
type PasteEvent struct {
	Text string
}

// FocusEvent reports terminal focus changes.
type FocusEvent struct {
	Focused bool
}

func (KeyEvent) inputEvent()   {}
func (MouseEvent) inputEvent() {}
func (PasteEvent) inputEvent() {}
func (FocusEvent) inputEvent() {}
