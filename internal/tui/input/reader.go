package input

import (
	"context"
	"io"
	"time"
)

const defaultEscTimeout = 25 * time.Millisecond

// ReaderOption configures a Reader.
type ReaderOption func(*readerConfig)

type readerConfig struct {
	escTimeout time.Duration
}

// WithEscTimeout sets how long Reader waits before treating a lone ESC byte as
// an Escape key instead of the start of an Alt chord or escape sequence.
func WithEscTimeout(d time.Duration) ReaderOption {
	return func(c *readerConfig) {
		c.escTimeout = d
	}
}

// Reader owns an input stream and publishes decoded events until its context is
// canceled or the stream ends.
type Reader struct {
	events <-chan Event
	errs   <-chan error
}

// NewReader starts a reader goroutine for r. If r implements io.Closer, it is
// closed when ctx is canceled so blocking reads can unblock promptly.
func NewReader(ctx context.Context, r io.Reader, opts ...ReaderOption) *Reader {
	cfg := readerConfig{escTimeout: defaultEscTimeout}
	for _, opt := range opts {
		opt(&cfg)
	}
	events := make(chan Event, 32)
	errs := make(chan error, 1)
	rr := &Reader{events: events, errs: errs}
	go runReader(ctx, r, cfg, events, errs)
	return rr
}

// Events returns decoded input events.
func (r *Reader) Events() <-chan Event {
	return r.events
}

// Errors returns terminal read errors. It is closed with Events.
func (r *Reader) Errors() <-chan error {
	return r.errs
}

func runReader(ctx context.Context, r io.Reader, cfg readerConfig, events chan<- Event, errs chan<- error) {
	defer close(events)
	defer close(errs)

	done := make(chan struct{})
	defer close(done)
	if closer, ok := r.(io.Closer); ok {
		go func() {
			select {
			case <-ctx.Done():
				_ = closer.Close()
			case <-done:
			}
		}()
	}

	type chunk struct {
		data []byte
		err  error
	}
	chunks := make(chan chunk, 1)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				cp := append([]byte(nil), buf[:n]...)
				select {
				case chunks <- chunk{data: cp}:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				select {
				case chunks <- chunk{err: err}:
				case <-ctx.Done():
				}
				return
			}
		}
	}()

	parser := NewParser()
	var timer *time.Timer
	var timerC <-chan time.Time
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timerC = nil
	}
	startTimer := func() {
		if cfg.escTimeout <= 0 {
			for _, ev := range parser.Flush() {
				sendEvent(ctx, events, ev)
			}
			return
		}
		if timer == nil {
			timer = time.NewTimer(cfg.escTimeout)
		} else {
			timer.Reset(cfg.escTimeout)
		}
		timerC = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			stopTimer()
			return
		case <-timerC:
			timerC = nil
			for _, ev := range parser.Flush() {
				if !sendEvent(ctx, events, ev) {
					return
				}
			}
		case c := <-chunks:
			if c.err != nil {
				if c.err != io.EOF {
					select {
					case errs <- c.err:
					default:
					}
				}
				for _, ev := range parser.Flush() {
					if !sendEvent(ctx, events, ev) {
						return
					}
				}
				return
			}
			stopTimer()
			for _, ev := range parser.Feed(c.data) {
				if !sendEvent(ctx, events, ev) {
					return
				}
			}
			if parser.pendingEscape() {
				startTimer()
			}
		}
	}
}

func sendEvent(ctx context.Context, events chan<- Event, ev Event) bool {
	select {
	case <-ctx.Done():
		return false
	case events <- ev:
		return true
	}
}

func (p *Parser) pendingEscape() bool {
	return len(p.buf) == 1 && p.buf[0] == esc && !p.pasting
}
