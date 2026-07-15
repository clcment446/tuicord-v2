package input

import (
	"reflect"
	"testing"
)

func TestParserKeys(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want []Event
	}{
		{"printable rune", []byte("é"), []Event{KeyEvent{Key: KeyRune, Rune: 'é'}}},
		{"enter", []byte("\r"), []Event{KeyEvent{Key: KeyEnter}}},
		{"ctrl c", []byte{0x03}, []Event{KeyEvent{Key: KeyRune, Mods: Ctrl, Rune: 'c'}}},
		{"arrow", []byte("\x1b[A"), []Event{KeyEvent{Key: KeyUp}}},
		{"back-tab", []byte("\x1b[Z"), []Event{KeyEvent{Key: KeyTab, Mods: Shift}}},
		{"modified arrow", []byte("\x1b[1;6D"), []Event{KeyEvent{Key: KeyLeft, Mods: Shift | Ctrl}}},
		{"alt arrows", []byte("\x1b[1;3D\x1b[1;3C"), []Event{
			KeyEvent{Key: KeyLeft, Mods: Alt},
			KeyEvent{Key: KeyRight, Mods: Alt},
		}},
		{"delete", []byte("\x1b[3~"), []Event{KeyEvent{Key: KeyDelete}}},
		{"ss3 function key", []byte("\x1bOP"), []Event{KeyEvent{Key: KeyF1}}},
		{"kitty ctrl a", []byte("\x1b[97;5u"), []Event{KeyEvent{Key: KeyRune, Mods: Ctrl, Rune: 'a'}}},
		{"kitty shift enter", []byte("\x1b[13;2u"), []Event{KeyEvent{Key: KeyEnter, Mods: Shift}}},
		{"kitty release", []byte("\x1b[13;1;3u"), []Event{KeyEvent{Key: KeyEnter, Release: true}}},
		{"alt rune", []byte("\x1bb"), []Event{KeyEvent{Key: KeyRune, Mods: Alt, Rune: 'b'}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			got := p.Feed(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Feed() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParserEscapeFlush(t *testing.T) {
	p := NewParser()
	if got := p.Feed([]byte{esc}); got != nil {
		t.Fatalf("Feed(ESC) = %#v, want pending", got)
	}
	got := p.Flush()
	want := []Event{KeyEvent{Key: KeyEsc}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Flush() = %#v, want %#v", got, want)
	}
}

func TestParserMouse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want MouseEvent
	}{
		{"press", "\x1b[<0;12;4M", MouseEvent{X: 11, Y: 3, Btn: ButtonLeft, Kind: MousePress}},
		{"release", "\x1b[<0;12;4m", MouseEvent{X: 11, Y: 3, Btn: ButtonNone, Kind: MouseRelease}},
		{"motion ctrl", "\x1b[<48;2;3M", MouseEvent{X: 1, Y: 2, Btn: ButtonLeft, Mods: Ctrl, Kind: MouseMotion}},
		{"wheel down", "\x1b[<65;1;1M", MouseEvent{X: 0, Y: 0, Btn: ButtonWheelDown, Kind: MouseWheel}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			got := p.Feed([]byte(tt.in))
			if len(got) != 1 {
				t.Fatalf("got %d events, want 1: %#v", len(got), got)
			}
			if got[0] != tt.want {
				t.Fatalf("mouse = %#v, want %#v", got[0], tt.want)
			}
		})
	}
}

func TestParserPaste(t *testing.T) {
	p := NewParser()
	if got := p.Feed([]byte("\x1b[200~hello")); got != nil {
		t.Fatalf("partial paste emitted %#v", got)
	}
	got := p.Feed([]byte("\nworld\x1b[201~x"))
	want := []Event{
		PasteEvent{Text: "hello\nworld"},
		KeyEvent{Key: KeyRune, Rune: 'x'},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Feed(paste) = %#v, want %#v", got, want)
	}
}

func TestParserFocusAndUnknown(t *testing.T) {
	p := NewParser()
	got := p.Feed([]byte("\x1b[I\x1b[O\x1b[999z"))
	want := []Event{FocusEvent{Focused: true}, FocusEvent{Focused: false}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Feed() = %#v, want %#v", got, want)
	}
}
