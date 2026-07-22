package ui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"awesomeProject/internal/tui/input"
)

// keyMatches reports whether ev matches a config key spec such as "ctrl+k",
// "esc", or a single letter. Matching is case-insensitive and ignores key
// releases. Only the modifiers and keys the client needs are recognized.
func keyMatches(ev input.KeyEvent, spec string) bool {
	if ev.Release || spec == "" {
		return false
	}
	spec = strings.TrimSpace(spec)

	var want input.Mod
	for {
		i := strings.IndexByte(spec, '+')
		if i < 0 {
			break
		}
		switch mod := spec[:i]; {
		case strings.EqualFold(mod, "ctrl"):
			want |= input.Ctrl
		case strings.EqualFold(mod, "shift"):
			want |= input.Shift
		case strings.EqualFold(mod, "alt"):
			want |= input.Alt
		case strings.EqualFold(mod, "super"):
			want |= input.Super
		case strings.EqualFold(mod, "hyper"):
			want |= input.Hyper
		case strings.EqualFold(mod, "meta"):
			want |= input.Meta
		default:
			return false
		}
		spec = spec[i+1:]
	}

	key, size := utf8.DecodeRuneInString(spec)
	const known = input.Shift | input.Alt | input.Ctrl | input.Super | input.Hyper | input.Meta
	if ev.Mods&(known&^input.Shift) != want&(known&^input.Shift) {
		return false
	}
	one := size == len(spec)
	upper := one && unicode.IsUpper(key)
	shift := want&input.Shift != 0
	if shift && ev.Mods&input.Shift == 0 || !shift && !upper && ev.Mods&input.Shift != 0 {
		return false
	}
	switch {
	case strings.EqualFold(spec, "esc"), strings.EqualFold(spec, "escape"):
		return ev.Key == input.KeyEsc
	case strings.EqualFold(spec, "tab"):
		return ev.Key == input.KeyTab
	case strings.EqualFold(spec, "enter"), strings.EqualFold(spec, "return"):
		return ev.Key == input.KeyEnter
	case strings.EqualFold(spec, "space"):
		return ev.Key == input.KeyRune && ev.Rune == ' '
	case strings.EqualFold(spec, "left"):
		return ev.Key == input.KeyLeft
	case strings.EqualFold(spec, "right"):
		return ev.Key == input.KeyRight
	case strings.EqualFold(spec, "up"):
		return ev.Key == input.KeyUp
	case strings.EqualFold(spec, "down"):
		return ev.Key == input.KeyDown
	default:
		if !one || ev.Key != input.KeyRune {
			return false
		}
		if shift {
			return unicode.ToLower(ev.Rune) == unicode.ToLower(key)
		}
		return ev.Rune == key
	}
}

func vimKey(spec, fallback string) string {
	if spec != "" {
		return spec
	}
	return fallback
}

func vimIns(ev input.KeyEvent, spec string) bool {
	spec = vimKey(spec, "i")
	return keyMatches(ev, spec) || spec == "i" && keyMatches(ev, "I")
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
