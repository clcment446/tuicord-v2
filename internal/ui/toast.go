package ui

import (
	"strings"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
	tuitext "awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

const (
	toastWidth     = 44
	toastCollapsed = 4
)

// Toast is a transient popup for recoverable errors.
type Toast struct {
	title    string
	detail   string
	styles   Styles
	expanded bool
}

func NewToast(title, detail string, styles Styles) *Toast {
	return &Toast{title: title, detail: detail, styles: styles}
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
			t.Toggle()
			return true
		case ev.Key == input.KeyRune && (ev.Rune == ' ' || ev.Rune == 'e'):
			t.Toggle()
			return true
		case ev.Key == input.KeyRune && (ev.Rune == 'x' || ev.Rune == 'd'):
			return true
		}
		return true
	case input.MouseEvent:
		if ev.Kind == input.MousePress && ev.Btn == input.ButtonLeft {
			t.Toggle()
		}
		return true
	default:
		return true
	}
}

func (t *Toast) wantsDismiss(ev tui.Event) bool {
	switch ev := ev.(type) {
	case input.KeyEvent:
		if ev.Release {
			return false
		}
		return ev.Key == input.KeyEsc ||
			(ev.Key == input.KeyRune && (ev.Rune == 'x' || ev.Rune == 'd'))
	case input.MouseEvent:
		return ev.Kind == input.MousePress && (ev.Btn == input.ButtonRight || ev.Btn == input.ButtonMiddle)
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

	style := screen.Style{Fg: t.styles.Text.Fg, Bg: screen.RGB(32, 35, 40)}
	titleStyle := screen.Style{Fg: t.styles.Error.Fg, Bg: style.Bg, Attrs: screen.Bold}
	mutedStyle := screen.Style{Fg: t.styles.Muted.Fg, Bg: style.Bg}

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
