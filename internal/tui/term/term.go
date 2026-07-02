// Package term owns the real terminal boundary for the TUI library.
//
// Higher layers should treat a terminal as a byte sink plus a resize source:
// raw mode setup, alternate-screen lifecycle, capability probing, signal
// handling, and restoration live here so drawing, input parsing, and widgets
// can stay pure and testable. Close is idempotent; use Run or defer Close
// immediately after Open so panics restore the user's shell.
package term

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	xterm "golang.org/x/term"
)

const (
	enterAltScreen        = "\x1b[?1049h"
	leaveAltScreen        = "\x1b[?1049l"
	enableBracketedPaste  = "\x1b[?2004h"
	disableBracketedPaste = "\x1b[?2004l"
	enableFocusEvents     = "\x1b[?1004h"
	disableFocusEvents    = "\x1b[?1004l"
	enableSGRMouse        = "\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h"
	disableSGRMouse       = "\x1b[?1003l\x1b[?1002l\x1b[?1000l\x1b[?1006l"
	hideCursor            = "\x1b[?25l"
	showCursor            = "\x1b[?25h"
)

// Size is a terminal size in display cells.
type Size struct {
	// Width is the number of terminal columns.
	Width int
	// Height is the number of terminal rows.
	Height int
}

// Capabilities describes terminal features the renderer and input layer can
// choose to use. Values are conservative: false means "do not rely on it".
type Capabilities struct {
	// TrueColor reports whether 24-bit color output is likely supported.
	TrueColor bool
	// KittyKeyboard reports whether the Kitty keyboard protocol appears usable.
	KittyKeyboard bool
	// SyncOutput reports whether synchronized output (CSI ?2026) is supported.
	SyncOutput bool
	// Color256 reports whether at least 256-color output is likely supported.
	Color256 bool
}

// Terminal is an open raw-mode terminal session.
type Terminal struct {
	file   *os.File
	fd     int
	state  *xterm.State
	caps   Capabilities
	resize chan Size
	sig    chan os.Signal
	done   chan struct{}
	wg     sync.WaitGroup
	once   sync.Once
	err    error
}

// Open switches the controlling terminal into raw mode, enters the alternate
// screen, enables modern input reporting, probes capabilities, and starts a
// coalesced SIGWINCH resize channel.
func Open() (*Terminal, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open tty: %w", err)
	}
	fd := int(f.Fd())
	state, err := xterm.MakeRaw(fd)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("make raw: %w", err)
	}

	t := &Terminal{
		file:   f,
		fd:     fd,
		state:  state,
		caps:   probeCapabilities(fd, os.Environ(), 30*time.Millisecond),
		resize: make(chan Size, 1),
		sig:    make(chan os.Signal, 1),
		done:   make(chan struct{}),
	}
	if _, err := f.WriteString(enterAltScreen + hideCursor + enableBracketedPaste + enableFocusEvents + enableSGRMouse); err != nil {
		t.Close()
		return nil, fmt.Errorf("initialize terminal: %w", err)
	}
	t.publishSize()
	signal.Notify(t.sig, syscall.SIGWINCH)
	t.wg.Add(1)
	go t.watchResizes()
	return t, nil
}

// Run opens a terminal, invokes fn, and always restores the terminal before
// returning or repanicking. It is the safest entry point for programs whose main
// loop owns the terminal for its whole lifetime.
func Run(fn func(*Terminal) error) (err error) {
	t, err := Open()
	if err != nil {
		return err
	}
	defer func() {
		closeErr := t.Close()
		if r := recover(); r != nil {
			panic(r)
		}
		if err == nil {
			err = closeErr
		}
	}()
	return fn(t)
}

// Caps returns the terminal capabilities detected at Open time.
func (t *Terminal) Caps() Capabilities {
	if t == nil {
		return Capabilities{}
	}
	return t.caps
}

// Resizes returns a channel that receives coalesced terminal sizes. The current
// size is sent once after Open when it can be read.
func (t *Terminal) Resizes() <-chan Size {
	return t.resize
}

// Write writes a complete frame to the terminal. Callers should batch escape
// sequences and cell output before calling Write.
func (t *Terminal) Write(frame []byte) (int, error) {
	if t == nil || t.file == nil {
		return 0, errors.New("terminal is closed")
	}
	return t.file.Write(frame)
}

// Read reads raw input bytes from the terminal. It lets the input.Reader own
// decoding while term remains the only package that touches the tty file.
func (t *Terminal) Read(p []byte) (int, error) {
	if t == nil || t.file == nil {
		return 0, errors.New("terminal is closed")
	}
	return t.file.Read(p)
}

// Size returns the terminal's current size.
func (t *Terminal) Size() (Size, error) {
	if t == nil {
		return Size{}, errors.New("terminal is closed")
	}
	return size(t.fd)
}

// Close restores the terminal. It is safe to call multiple times.
func (t *Terminal) Close() error {
	if t == nil {
		return nil
	}
	t.once.Do(func() {
		close(t.done)
		signal.Stop(t.sig)
		t.wg.Wait()
		if t.file != nil {
			_, writeErr := t.file.WriteString(disableSGRMouse + disableFocusEvents + disableBracketedPaste + showCursor + leaveAltScreen)
			restoreErr := xterm.Restore(t.fd, t.state)
			closeErr := t.file.Close()
			t.err = errors.Join(writeErr, restoreErr, closeErr)
		}
		close(t.resize)
	})
	return t.err
}

func (t *Terminal) watchResizes() {
	defer t.wg.Done()
	for {
		select {
		case <-t.done:
			return
		case <-t.sig:
			t.publishSize()
		}
	}
}

func (t *Terminal) publishSize() {
	sz, err := size(t.fd)
	if err != nil {
		return
	}
	select {
	case t.resize <- sz:
	default:
		select {
		case <-t.resize:
		default:
		}
		select {
		case t.resize <- sz:
		default:
		}
	}
}

func size(fd int) (Size, error) {
	w, h, err := xterm.GetSize(fd)
	if err != nil {
		return Size{}, err
	}
	return Size{Width: w, Height: h}, nil
}

func envMap(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, kv := range environ {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			out[k] = v
		}
	}
	return out
}
