package ui

import (
	"strings"

	"awesomeProject/internal/tui/input"
)

// keyMatches reports whether ev matches a config key spec such as "ctrl+k",
// "esc", or a single letter. Matching is case-insensitive and ignores key
// releases. Only the modifiers and keys the client needs are recognized.
func keyMatches(ev input.KeyEvent, spec string) bool {
	if ev.Release || spec == "" {
		return false
	}
	spec = strings.ToLower(strings.TrimSpace(spec))

	wantCtrl := false
	for {
		switch {
		case strings.HasPrefix(spec, "ctrl+"):
			wantCtrl = true
			spec = spec[len("ctrl+"):]
		default:
			goto match
		}
	}

match:
	if wantCtrl != (ev.Mods&input.Ctrl != 0) {
		return false
	}
	switch spec {
	case "esc", "escape":
		return ev.Key == input.KeyEsc
	case "tab":
		return ev.Key == input.KeyTab
	case "enter", "return":
		return ev.Key == input.KeyEnter
	default:
		// Single character key.
		return ev.Key == input.KeyRune && strings.ToLower(string(ev.Rune)) == spec
	}
}
