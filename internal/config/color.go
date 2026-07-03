package config

import (
	"fmt"
	"strconv"
	"strings"

	"awesomeProject/internal/tui/screen"
)

// ParseColor converts a "#rrggbb" (or "rrggbb") hex string into a screen.Color.
// An empty string yields the terminal default color.
func ParseColor(hex string) (screen.Color, error) {
	hex = strings.TrimSpace(hex)
	if hex == "" {
		return screen.Color{}, nil
	}
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return screen.Color{}, fmt.Errorf("invalid hex color %q: want 6 digits", hex)
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return screen.Color{}, fmt.Errorf("invalid hex color %q: %w", hex, err)
	}
	return screen.RGB(uint8(v>>16), uint8(v>>8), uint8(v)), nil
}

// mustColor parses hex, falling back to the terminal default on error.
func mustColor(hex string) screen.Color {
	c, err := ParseColor(hex)
	if err != nil {
		return screen.Color{}
	}
	return c
}

// Styles resolves configured colors into ready-to-use screen styles. The
// configured background is painted onto every foreground style so panels share
// a cohesive backdrop rather than showing the terminal default through gaps.
func (c Colors) Styles() ColorStyles {
	bg := mustColor(c.Background)
	styles := ColorStyles{
		Background: bg,
		Text:       screen.Style{Fg: mustColor(c.Text)},
		Muted:      mutedStyle(c.Muted),
		Accent:     screen.Style{Fg: mustColor(c.Accent), Attrs: screen.Bold},
		Selection:  selectionStyle(c.Selection),
		Border:     screen.Style{Fg: mustColor(c.Border)},
		Error:      screen.Style{Fg: mustColor(c.Error)},
	}
	if bg.Set() {
		styles.Text.Bg = bg
		styles.Muted.Bg = bg
		styles.Accent.Bg = bg
		styles.Border.Bg = bg
		styles.Error.Bg = bg
	}
	return styles
}

func mutedStyle(hex string) screen.Style {
	style := screen.Style{Fg: mustColor(hex)}
	if strings.TrimSpace(hex) == "" {
		style.Attrs = screen.Dim
	}
	return style
}

func selectionStyle(hex string) screen.Style {
	style := screen.Style{Bg: mustColor(hex)}
	if strings.TrimSpace(hex) == "" {
		style.Attrs = screen.Reverse
	}
	return style
}

// ColorStyles is a resolved palette of screen styles.
type ColorStyles struct {
	// Background is the base fill color painted behind all panels.
	Background screen.Color
	Text       screen.Style
	Muted      screen.Style
	Accent     screen.Style
	Selection  screen.Style
	Border     screen.Style
	Error      screen.Style
}

// ThemeStyles is kept as a source-compatible alias for older callers.
type ThemeStyles = ColorStyles
