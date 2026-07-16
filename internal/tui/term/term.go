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

	"golang.org/x/sys/unix"
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
	// XPixel and YPixel are the terminal's total size in pixels, as reported
	// by the kernel. Many terminals (and tmux) report zero; treat that as
	// "unknown" rather than as a real size. Dividing by Width and Height gives
	// the pixel size of one cell, which pixel-addressed protocols such as
	// Kitty graphics need to size images.
	XPixel int
	YPixel int
}

// CellPixels returns the pixel size of a single terminal cell. ok is false when
// the terminal does not report pixel dimensions, in which case callers should
// fall back to a conventional cell size rather than trusting the zero values.
func (s Size) CellPixels() (w, h int, ok bool) {
	if s.XPixel <= 0 || s.YPixel <= 0 || s.Width <= 0 || s.Height <= 0 {
		return 0, 0, false
	}
	w = s.XPixel / s.Width
	h = s.YPixel / s.Height
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// Options controls terminal capabilities enabled for a session.
type Options struct {
	// Mouse controls SGR mouse reporting. It defaults to true when using Run.
	Mouse bool
}

type mouseSequences struct {
	enable  string
	disable string
}

func terminalMouseSequences(opts Options) mouseSequences {
	if !opts.Mouse {
		return mouseSequences{}
	}
	return mouseSequences{enable: enableSGRMouse, disable: disableSGRMouse}
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
	file         *os.File
	fd           int
	state        *xterm.State
	caps         Capabilities
	resize       chan Size
	sig          chan os.Signal
	done         chan struct{}
	wg           sync.WaitGroup
	once         sync.Once
	err          error
	mouseEnabled bool
}

// Open switches the controlling terminal into raw mode, enters the alternate
// screen, enables modern input reporting, probes capabilities, and starts a
// coalesced SIGWINCH resize channel.
func Open() (*Terminal, error) {
	return open(Options{Mouse: true})
}

func open(opts Options) (*Terminal, error) {
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
		file:         f,
		fd:           fd,
		state:        state,
		caps:         probeCapabilities(fd, os.Environ(), 30*time.Millisecond),
		resize:       make(chan Size, 1),
		sig:          make(chan os.Signal, 1),
		done:         make(chan struct{}),
		mouseEnabled: opts.Mouse,
	}
	mouse := terminalMouseSequences(opts).enable
	if _, err := f.WriteString(enterAltScreen + hideCursor + enableBracketedPaste + enableFocusEvents + mouse); err != nil {
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
	return RunWithOptions(Options{Mouse: true}, fn)
}

// RunWithOptions opens a configured terminal session and always restores it.
func RunWithOptions(opts Options, fn func(*Terminal) error) (err error) {
	t, err := open(opts)
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

// ProbeSize reports the controlling terminal's size without opening a session.
// It neither enters raw mode nor touches the alternate screen, so it is safe to
// call during startup before the event loop runs — for example to learn the
// cell pixel size that graphics protocols need.
func ProbeSize() (Size, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return Size{}, fmt.Errorf("open tty: %w", err)
	}
	defer f.Close()
	return size(int(f.Fd()))
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
			mouse := terminalMouseSequences(Options{Mouse: t.mouseEnabled}).disable
			_, writeErr := t.file.WriteString(mouse + disableFocusEvents + disableBracketedPaste + showCursor + leaveAltScreen)
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

// size reports the terminal size in cells and, when the terminal supplies it,
// in pixels. It uses TIOCGWINSZ directly because x/term's GetSize discards the
// pixel fields, which pixel-addressed graphics protocols need.
func size(fd int) (Size, error) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		// Fall back to x/term so a platform without the ioctl still reports cells.
		w, h, gerr := xterm.GetSize(fd)
		if gerr != nil {
			return Size{}, err
		}
		return Size{Width: w, Height: h}, nil
	}
	return Size{
		Width:  int(ws.Col),
		Height: int(ws.Row),
		XPixel: int(ws.Xpixel),
		YPixel: int(ws.Ypixel),
	}, nil
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
