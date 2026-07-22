package input

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// KeyMatches reports whether ev matches a key specification such as "ctrl+k",
// "shift+tab", "esc", or a single rune. Releases and empty specifications do
// not match.
func KeyMatches(ev KeyEvent, spec string) bool {
	if ev.Release || spec == "" {
		return false
	}
	spec = strings.TrimSpace(spec)

	var want Mod
	for {
		i := strings.IndexByte(spec, '+')
		if i < 0 {
			break
		}
		switch mod := spec[:i]; {
		case strings.EqualFold(mod, "ctrl"):
			want |= Ctrl
		case strings.EqualFold(mod, "shift"):
			want |= Shift
		case strings.EqualFold(mod, "alt"):
			want |= Alt
		case strings.EqualFold(mod, "super"):
			want |= Super
		case strings.EqualFold(mod, "hyper"):
			want |= Hyper
		case strings.EqualFold(mod, "meta"):
			want |= Meta
		default:
			return false
		}
		spec = spec[i+1:]
	}

	key, size := utf8.DecodeRuneInString(spec)
	const known = Shift | Alt | Ctrl | Super | Hyper | Meta
	if ev.Mods&(known&^Shift) != want&(known&^Shift) {
		return false
	}
	one := size == len(spec)
	upper := one && unicode.IsUpper(key)
	shift := want&Shift != 0
	if shift && ev.Mods&Shift == 0 || !shift && !upper && ev.Mods&Shift != 0 {
		return false
	}

	switch {
	case strings.EqualFold(spec, "esc"), strings.EqualFold(spec, "escape"):
		return ev.Key == KeyEsc
	case strings.EqualFold(spec, "tab"):
		return ev.Key == KeyTab
	case strings.EqualFold(spec, "enter"), strings.EqualFold(spec, "return"):
		return ev.Key == KeyEnter
	case strings.EqualFold(spec, "space"):
		return ev.Key == KeyRune && ev.Rune == ' '
	case strings.EqualFold(spec, "left"):
		return ev.Key == KeyLeft
	case strings.EqualFold(spec, "right"):
		return ev.Key == KeyRight
	case strings.EqualFold(spec, "up"):
		return ev.Key == KeyUp
	case strings.EqualFold(spec, "down"):
		return ev.Key == KeyDown
	default:
		if !one || ev.Key != KeyRune {
			return false
		}
		if shift {
			return unicode.ToLower(ev.Rune) == unicode.ToLower(key)
		}
		return ev.Rune == key
	}
}
