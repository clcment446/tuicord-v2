package ui

import (
	"unicode"

	"awesomeProject/internal/tui/input"
)

// keyMatches reports whether ev matches a config key spec such as "ctrl+k",
// "esc", or a single letter. Matching is case-insensitive and ignores key
// releases. Only the modifiers and keys the client needs are recognized.
func keyMatches(ev input.KeyEvent, spec string) bool {
	return input.KeyMatches(ev, spec)
}

func vimIns(ev input.KeyEvent, spec string) bool {
	return spec != "" && (keyMatches(ev, spec) || spec == "i" && keyMatches(ev, "I"))
}

func vimAct(ev input.KeyEvent, spec string) bool {
	if keyMatches(ev, spec) {
		return true
	}
	if ev.Key != input.KeyRune || ev.Mods&(input.Ctrl|input.Alt|input.Super|input.Hyper|input.Meta) != 0 {
		return false
	}
	ev.Rune = unicode.ToLower(ev.Rune)
	ev.Mods &^= input.Shift
	return keyMatches(ev, spec)
}
