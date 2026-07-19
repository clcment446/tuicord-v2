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

	var want input.Mod
	for {
		switch {
		case strings.HasPrefix(spec, "ctrl+"):
			want |= input.Ctrl
			spec = spec[len("ctrl+"):]
		case strings.HasPrefix(spec, "shift+"):
			want |= input.Shift
			spec = spec[len("shift+"):]
		case strings.HasPrefix(spec, "alt+"):
			want |= input.Alt
			spec = spec[len("alt+"):]
		case strings.HasPrefix(spec, "super+"):
			want |= input.Super
			spec = spec[len("super+"):]
		case strings.HasPrefix(spec, "hyper+"):
			want |= input.Hyper
			spec = spec[len("hyper+"):]
		case strings.HasPrefix(spec, "meta+"):
			want |= input.Meta
			spec = spec[len("meta+"):]
		default:
			goto match
		}
	}

match:
	const known = input.Shift | input.Alt | input.Ctrl | input.Super | input.Hyper | input.Meta
	if ev.Mods&known != want {
		return false
	}
	switch spec {
	case "esc", "escape":
		return ev.Key == input.KeyEsc
	case "tab":
		return ev.Key == input.KeyTab
	case "enter", "return":
		return ev.Key == input.KeyEnter
	case "space":
		return ev.Key == input.KeyRune && ev.Rune == ' '
	case "left":
		return ev.Key == input.KeyLeft
	case "right":
		return ev.Key == input.KeyRight
	case "up":
		return ev.Key == input.KeyUp
	case "down":
		return ev.Key == input.KeyDown
	default:
		// Single character key.
		return ev.Key == input.KeyRune && strings.ToLower(string(ev.Rune)) == spec
	}
}
