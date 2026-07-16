package input

import (
	"strings"
	"unicode/utf8"
)

const (
	esc       = byte(0x1b)
	pasteEnd  = "\x1b[201~"
	pasteOpen = "\x1b[200~"
)

// Parser incrementally decodes terminal input bytes into events.
type Parser struct {
	buf     []byte
	events  []Event
	pasting bool
	paste   strings.Builder
}

// NewParser returns an empty parser.
func NewParser() *Parser {
	return &Parser{}
}

// Feed appends bytes to the parser and returns all complete events decoded so
// far. An isolated ESC is held until more bytes arrive or Flush is called.
func (p *Parser) Feed(b []byte) []Event {
	p.buf = append(p.buf, b...)
	p.parse(false)
	return p.drain()
}

// Flush resolves incomplete input. In practice Reader calls this when the ESC
// timeout expires, producing a literal Escape key for a lone ESC byte.
func (p *Parser) Flush() []Event {
	p.parse(true)
	if len(p.buf) > 0 {
		p.buf = p.buf[:0]
	}
	return p.drain()
}

func (p *Parser) drain() []Event {
	if len(p.events) == 0 {
		return nil
	}
	out := append([]Event(nil), p.events...)
	p.events = p.events[:0]
	return out
}

func (p *Parser) emit(ev Event) {
	p.events = append(p.events, ev)
}

func (p *Parser) parse(flush bool) {
	for {
		if p.pasting {
			if !p.parsePaste(flush) {
				return
			}
			continue
		}
		if len(p.buf) == 0 {
			return
		}
		if p.buf[0] == esc {
			if !p.parseEscape(flush) {
				return
			}
			continue
		}
		if !p.parsePlain(flush) {
			return
		}
	}
}

func (p *Parser) parsePaste(flush bool) bool {
	s := string(p.buf)
	if i := strings.Index(s, pasteEnd); i >= 0 {
		p.paste.WriteString(s[:i])
		p.emit(PasteEvent{Text: p.paste.String()})
		p.paste.Reset()
		p.pasting = false
		p.buf = p.buf[i+len(pasteEnd):]
		return true
	}
	keep := longestSuffixPrefix(s, pasteEnd)
	if flush {
		keep = 0
	}
	if len(p.buf) > keep {
		p.paste.Write(p.buf[:len(p.buf)-keep])
		p.buf = p.buf[len(p.buf)-keep:]
	}
	return flush
}

func (p *Parser) parsePlain(flush bool) bool {
	b := p.buf[0]
	switch b {
	case '\r', '\n':
		p.emit(KeyEvent{Key: KeyEnter})
		p.buf = p.buf[1:]
		return true
	case '\t':
		p.emit(KeyEvent{Key: KeyTab})
		p.buf = p.buf[1:]
		return true
	case 0x7f, 0x08:
		p.emit(KeyEvent{Key: KeyBackspace})
		p.buf = p.buf[1:]
		return true
	}
	if b < 0x20 {
		p.emit(controlKey(b))
		p.buf = p.buf[1:]
		return true
	}
	r, n := utf8.DecodeRune(p.buf)
	if r == utf8.RuneError && n == 1 && !utf8.FullRune(p.buf) && !flush {
		return false
	}
	if r == utf8.RuneError && n == 1 {
		p.buf = p.buf[1:]
		return true
	}
	p.emit(KeyEvent{Key: KeyRune, Rune: r})
	p.buf = p.buf[n:]
	return true
}

func (p *Parser) parseEscape(flush bool) bool {
	if len(p.buf) == 1 {
		if !flush {
			return false
		}
		p.emit(KeyEvent{Key: KeyEsc})
		p.buf = p.buf[1:]
		return true
	}
	if p.buf[1] != '[' && p.buf[1] != 'O' {
		start := len(p.events)
		p.buf = p.buf[1:]
		if !p.parsePlain(true) {
			p.emit(KeyEvent{Key: KeyEsc})
		} else if len(p.events) > start {
			if key, ok := p.events[len(p.events)-1].(KeyEvent); ok {
				key.Mods |= Alt
				p.events[len(p.events)-1] = key
			}
		}
		return true
	}
	if p.buf[1] == 'O' {
		return p.parseSS3(flush)
	}
	return p.parseCSI(flush)
}

func (p *Parser) parseSS3(flush bool) bool {
	if len(p.buf) < 3 {
		if flush {
			p.buf = nil
			return true
		}
		return false
	}
	if key, ok := ss3Key(p.buf[2]); ok {
		p.emit(KeyEvent{Key: key})
	}
	p.buf = p.buf[3:]
	return true
}

func (p *Parser) parseCSI(flush bool) bool {
	end := -1
	for i := 2; i < len(p.buf); i++ {
		if p.buf[i] >= 0x40 && p.buf[i] <= 0x7e {
			end = i
			break
		}
	}
	if end < 0 {
		if flush {
			p.buf = nil
			return true
		}
		return false
	}
	seq := string(p.buf[2:end])
	final := p.buf[end]
	p.buf = p.buf[end+1:]

	full := "\x1b[" + seq + string(final)
	if full == pasteOpen {
		p.pasting = true
		return true
	}
	switch full {
	case "\x1b[I":
		p.emit(FocusEvent{Focused: true})
		return true
	case "\x1b[O":
		p.emit(FocusEvent{Focused: false})
		return true
	}
	if final == 'M' || final == 'm' {
		p.parseMouse(seq, final == 'm')
		return true
	}
	if final == 'u' {
		p.parseKittyKey(seq)
		return true
	}
	p.parseLegacyCSI(seq, final)
	return true
}

func (p *Parser) parseMouse(seq string, release bool) {
	if !strings.HasPrefix(seq, "<") {
		return
	}
	parts := strings.Split(seq[1:], ";")
	if len(parts) != 3 {
		return
	}
	cb, ok1 := atoi(parts[0])
	x, ok2 := atoi(parts[1])
	y, ok3 := atoi(parts[2])
	if !ok1 || !ok2 || !ok3 {
		return
	}
	btn, kind := mouseButton(cb, release)
	p.emit(MouseEvent{X: max(x-1, 0), Y: max(y-1, 0), Btn: btn, Mods: mouseMods(cb), Kind: kind})
}

func (p *Parser) parseKittyKey(seq string) {
	parts := strings.Split(seq, ";")
	code, ok := atoi(firstSubParam(parts[0]))
	if !ok {
		return
	}
	var mods Mod
	if len(parts) > 1 {
		if m, ok := atoi(firstSubParam(parts[1])); ok {
			mods = kittyMods(m)
		}
	}
	release := false
	if len(parts) > 2 {
		if ev, ok := atoi(firstSubParam(parts[2])); ok {
			release = ev == 3
		}
	}
	key := kittyNamedKey(code)
	if key == KeyRune && code >= 0 && code <= utf8.MaxRune {
		p.emit(KeyEvent{Key: KeyRune, Mods: mods, Rune: rune(code), Release: release})
		return
	}
	p.emit(KeyEvent{Key: key, Mods: mods, Release: release})
}

func (p *Parser) parseLegacyCSI(seq string, final byte) {
	if final == 'Z' {
		// CSI Z is the terminal-standard back-tab (Shift+Tab) sequence.
		p.emit(KeyEvent{Key: KeyTab, Mods: Shift})
		return
	}
	if key, ok := csiFinalKey(final); ok {
		parts := splitParams(seq)
		mods := Mod(0)
		if len(parts) > 1 {
			mods = legacyMods(parts[1])
		}
		p.emit(KeyEvent{Key: key, Mods: mods})
		return
	}
	if final != '~' {
		return
	}
	parts := splitParams(seq)
	if len(parts) == 0 {
		return
	}
	key := tildeKey(parts[0])
	if key == KeyUnknown {
		return
	}
	var mods Mod
	if len(parts) > 1 {
		mods = legacyMods(parts[1])
	}
	p.emit(KeyEvent{Key: key, Mods: mods})
}
