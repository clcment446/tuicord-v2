package ui

import (
	"strings"
	"time"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
	tuitext "awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

const notificationTTL = 7 * time.Second

const (
	toastWidth     = 44
	toastCollapsed = 4
)

// Toast is a transient popup for recoverable errors.
type Toast struct {
	title      string
	detail     string
	styles     Styles
	expanded   bool
	expiresAt  time.Time
	onActivate func()
	bounds     screen.Rect
}

func NewToast(title, detail string, styles Styles) *Toast {
	return &Toast{title: title, detail: detail, styles: styles}
}

// SetTTL makes the toast auto-dismiss after d. It stays until dismissed if d is
// zero or negative. Expanding the toast (Enter) cancels the auto-dismiss so a
// user reading it isn't interrupted.
func (t *Toast) SetTTL(d time.Duration) *Toast {
	if t != nil && d > 0 {
		t.expiresAt = time.Now().Add(d)
	}
	return t
}

// expired reports whether the toast's auto-dismiss deadline has passed.
func (t *Toast) expired(now time.Time) bool {
	return t != nil && !t.expanded && !t.expiresAt.IsZero() && !now.Before(t.expiresAt)
}

func newExpiringToast(title, detail string, styles Styles, now time.Time) *Toast {
	toast := NewToast(title, detail, styles)
	toast.expiresAt = now.Add(notificationTTL)
	return toast
}

func (t *Toast) Expired(now time.Time) bool {
	return t != nil && t.expired(now)
}

func (t *Toast) Expanded() bool {
	return t != nil && t.expanded
}

func (t *Toast) Dismissed() bool {
	return t == nil
}

func (t *Toast) Toggle() {
	if t != nil {
		t.expanded = !t.expanded
	}
}

func (t *Toast) Handle(ev tui.Event) bool {
	if t == nil {
		return false
	}
	switch ev := ev.(type) {
	case input.KeyEvent:
		if ev.Release {
			return true
		}
		switch {
		case ev.Key == input.KeyEsc:
			return true
		case ev.Key == input.KeyEnter:
			if t.onActivate != nil {
				t.onActivate()
				return true
			}
			t.Toggle()
			return true
		case ev.Key == input.KeyRune && (ev.Rune == ' ' || ev.Rune == 'e'):
			t.Toggle()
			return true
		case ev.Key == input.KeyRune && (ev.Rune == 'x' || ev.Rune == 'd'):
			return true
		}
		return ev.Key == input.KeyEsc || ev.Key == input.KeyEnter ||
			(ev.Key == input.KeyRune && (ev.Rune == ' ' || ev.Rune == 'e' || ev.Rune == 'x' || ev.Rune == 'd'))
	case input.MouseEvent:
		if ev.Kind != input.MousePress || t.onActivate == nil || !t.contains(ev.X, ev.Y) {
			return false
		}
		t.onActivate()
		return true
	default:
		return false
	}
}

func (t *Toast) contains(x, y int) bool {
	return t != nil && x >= t.bounds.X && y >= t.bounds.Y && x < t.bounds.X+t.bounds.W && y < t.bounds.Y+t.bounds.H
}

func (t *Toast) wantsDismiss(ev tui.Event) bool {
	switch ev := ev.(type) {
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		return ev.Key == input.KeyEsc ||
			(ev.Key == input.KeyEnter && t.onActivate != nil) ||
			(ev.Key == input.KeyRune && (ev.Rune == 'x' || ev.Rune == 'd'))
	case input.MouseEvent:
		return ev.Kind == input.MousePress && t.onActivate != nil && t.contains(ev.X, ev.Y)
	default:
		return false
	}
}

func (t *Toast) Draw(r screen.Region) {
	if t == nil || r.Width() <= 0 || r.Height() <= 0 {
		return
	}
	width := min(toastWidth, r.Width())
	height := t.height(width, r.Height())
	x0 := max(r.Width()-width-1, 0)
	y0 := max(r.Height()-height-1, 0)
	t.drawAt(r, x0, y0, width, height)
}

func (t *Toast) drawAt(r screen.Region, x0, y0, width, height int) {
	if t == nil || width <= 0 || height <= 0 {
		return
	}
	t.bounds = screen.Rect{X: x0, Y: y0, W: width, H: height}

	style := t.styles.Cell("toast")
	if !style.Bg.Set() {
		style.Bg = t.styles.Cell("background").Bg
	}
	titleStyle := mergeStyle(style, t.styles.Cell("toast.title"))
	mutedStyle := mergeStyle(style, t.styles.Cell("toast.detail"))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r.Set(x0+x, y0+y, screen.Cell{Content: " ", Style: style})
		}
	}

	drawText(r, x0+1, y0, tuitext.Truncate(t.title, max(width-2, 0), tuitext.Ellipsis), titleStyle)
	lines := t.lines(max(width-2, 1), max(height-2, 0))
	for i, line := range lines {
		drawText(r, x0+1, y0+1+i, line, style)
	}
	hint := "Enter expand  x dismiss"
	if t.expanded {
		hint = "Enter collapse  x dismiss"
	}
	drawText(r, x0+1, y0+height-1, tuitext.Truncate(hint, max(width-2, 0), tuitext.Ellipsis), mutedStyle)
}

func (t *Toast) height(width, maxHeight int) int {
	if t == nil || maxHeight <= 0 {
		return 0
	}
	if !t.expanded {
		return min(toastCollapsed, maxHeight)
	}
	lines := len(t.lines(max(width-2, 1), maxHeight-2))
	return min(max(lines+2, toastCollapsed), maxHeight)
}

func (t *Toast) lines(width, maxLines int) []string {
	if t == nil || maxLines <= 0 {
		return nil
	}
	detail := strings.TrimSpace(t.detail)
	if detail == "" {
		detail = "Unknown error"
	}
	lines := tuitext.Wrap(detail, width)
	if !t.expanded && len(lines) > 1 {
		lines = lines[:1]
		lines[0] = tuitext.Truncate(lines[0], width, tuitext.Ellipsis)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		if len(lines) > 0 {
			lines[len(lines)-1] = tuitext.Truncate(lines[len(lines)-1], width, tuitext.Ellipsis)
		}
	}
	return lines
}
