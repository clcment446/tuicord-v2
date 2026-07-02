package input

import (
	"strconv"
	"strings"
)

func controlKey(b byte) KeyEvent {
	if b == 0x1b {
		return KeyEvent{Key: KeyEsc}
	}
	r := rune(b + 0x60)
	if b == 0 {
		r = ' '
	}
	return KeyEvent{Key: KeyRune, Mods: Ctrl, Rune: r}
}

func csiFinalKey(final byte) (Key, bool) {
	switch final {
	case 'A':
		return KeyUp, true
	case 'B':
		return KeyDown, true
	case 'C':
		return KeyRight, true
	case 'D':
		return KeyLeft, true
	case 'H':
		return KeyHome, true
	case 'F':
		return KeyEnd, true
	}
	return KeyUnknown, false
}

func ss3Key(final byte) (Key, bool) {
	switch final {
	case 'P':
		return KeyF1, true
	case 'Q':
		return KeyF2, true
	case 'R':
		return KeyF3, true
	case 'S':
		return KeyF4, true
	}
	return csiFinalKey(final)
}

func tildeKey(n int) Key {
	switch n {
	case 1, 7:
		return KeyHome
	case 2:
		return KeyInsert
	case 3:
		return KeyDelete
	case 4, 8:
		return KeyEnd
	case 5:
		return KeyPageUp
	case 6:
		return KeyPageDown
	case 11:
		return KeyF1
	case 12:
		return KeyF2
	case 13:
		return KeyF3
	case 14:
		return KeyF4
	case 15:
		return KeyF5
	case 17:
		return KeyF6
	case 18:
		return KeyF7
	case 19:
		return KeyF8
	case 20:
		return KeyF9
	case 21:
		return KeyF10
	case 23:
		return KeyF11
	case 24:
		return KeyF12
	}
	return KeyUnknown
}

func kittyNamedKey(code int) Key {
	switch code {
	case 9:
		return KeyTab
	case 13:
		return KeyEnter
	case 27:
		return KeyEsc
	case 127:
		return KeyBackspace
	}
	return KeyRune
}

func mouseButton(cb int, release bool) (Button, MouseKind) {
	if release {
		return ButtonNone, MouseRelease
	}
	if cb&64 != 0 {
		switch cb & 3 {
		case 0:
			return ButtonWheelUp, MouseWheel
		case 1:
			return ButtonWheelDown, MouseWheel
		case 2:
			return ButtonWheelLeft, MouseWheel
		case 3:
			return ButtonWheelRight, MouseWheel
		}
	}
	kind := MousePress
	if cb&32 != 0 {
		kind = MouseMotion
	}
	switch cb & 3 {
	case 0:
		return ButtonLeft, kind
	case 1:
		return ButtonMiddle, kind
	case 2:
		return ButtonRight, kind
	}
	return ButtonNone, kind
}

func mouseMods(cb int) Mod {
	var m Mod
	if cb&4 != 0 {
		m |= Shift
	}
	if cb&8 != 0 {
		m |= Alt
	}
	if cb&16 != 0 {
		m |= Ctrl
	}
	return m
}

func kittyMods(v int) Mod {
	if v <= 1 {
		return 0
	}
	return modifierBits(v - 1)
}

func legacyMods(v int) Mod {
	if v <= 1 {
		return 0
	}
	return modifierBits(v - 1)
}

func modifierBits(bits int) Mod {
	var m Mod
	if bits&1 != 0 {
		m |= Shift
	}
	if bits&2 != 0 {
		m |= Alt
	}
	if bits&4 != 0 {
		m |= Ctrl
	}
	if bits&8 != 0 {
		m |= Super
	}
	if bits&16 != 0 {
		m |= Hyper
	}
	if bits&32 != 0 {
		m |= Meta
	}
	return m
}

func splitParams(s string) []int {
	if s == "" {
		return nil
	}
	raw := strings.Split(s, ";")
	out := make([]int, 0, len(raw))
	for _, part := range raw {
		n, ok := atoi(firstSubParam(part))
		if !ok {
			return out
		}
		out = append(out, n)
	}
	return out
}

func firstSubParam(s string) string {
	first, _, _ := strings.Cut(s, ":")
	return first
}

func atoi(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func longestSuffixPrefix(s, prefix string) int {
	maxLen := min(len(s), len(prefix)-1)
	for n := maxLen; n > 0; n-- {
		if strings.HasPrefix(prefix, s[len(s)-n:]) {
			return n
		}
	}
	return 0
}
