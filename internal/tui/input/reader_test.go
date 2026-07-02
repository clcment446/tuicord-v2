package input

import (
	"context"
	"io"
	"reflect"
	"testing"
	"time"
)

func TestReaderFlushesLoneEscape(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pr, pw := io.Pipe()
	r := NewReader(ctx, pr, WithEscTimeout(time.Millisecond))
	if _, err := pw.Write([]byte{esc}); err != nil {
		t.Fatal(err)
	}
	got := <-r.Events()
	want := KeyEvent{Key: KeyEsc}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event = %#v, want %#v", got, want)
	}
}

func TestReaderKeepsAltChordTogether(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pr, pw := io.Pipe()
	r := NewReader(ctx, pr, WithEscTimeout(50*time.Millisecond))
	if _, err := pw.Write([]byte("\x1bx")); err != nil {
		t.Fatal(err)
	}
	got := <-r.Events()
	want := KeyEvent{Key: KeyRune, Mods: Alt, Rune: 'x'}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event = %#v, want %#v", got, want)
	}
}
