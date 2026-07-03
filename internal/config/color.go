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

// Styles resolves the theme's hex colors into ready-to-use screen styles.
func (t Theme) Styles() ThemeStyles {
	return ThemeStyles{
		Text:      screen.Style{Fg: mustColor(t.Text)},
		Muted:     screen.Style{Fg: mustColor(t.Muted)},
		Accent:    screen.Style{Fg: mustColor(t.Accent), Attrs: screen.Bold},
		Selection: screen.Style{Bg: mustColor(t.Selection)},
		Border:    screen.Style{Fg: mustColor(t.Border)},
		Error:     screen.Style{Fg: mustColor(t.Error)},
	}
}

// ThemeStyles is a resolved palette of screen styles.
type ThemeStyles struct {
	Text      screen.Style
	Muted     screen.Style
	Accent    screen.Style
	Selection screen.Style
	Border    screen.Style
	Error     screen.Style
}
