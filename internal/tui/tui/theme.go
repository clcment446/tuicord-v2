package tui

import "awesomeProject/internal/tui/screen"

// Theme is a small palette of styles the application applies to its widgets.
//
// Widgets are styled explicitly by the caller, but the runtime does paint the
// theme Background across the whole buffer before drawing so panel gaps and
// dividers share the palette rather than showing the terminal default. Carrying
// the theme on the App gives a single place for configuration (loaded from
// disk, say) to reach the code that builds the widget tree.
type Theme struct {
	// Background is painted behind the whole screen each frame. The zero value
	// leaves the terminal default background in place.
	Background screen.Color
	// Text is the default style for body text.
	Text screen.Style
	// Muted is a dimmed style for secondary text.
	Muted screen.Style
	// Accent highlights active or selected elements.
	Accent screen.Style
	// Selection styles the focused row in a list.
	Selection screen.Style
	// Border styles panel borders.
	Border screen.Style
	// Error styles failure states, such as a message that failed to send.
	Error screen.Style
}

// WithTheme sets the App's theme. Widgets read it via App.Theme when the tree
// is built.
func WithTheme(theme Theme) Option {
	return func(a *App) {
		a.theme = theme
	}
}

// SetTheme safely replaces the live application theme and invalidates the
// frame so the background and toolkit-level palette repaint immediately.
func (a *App) SetTheme(theme Theme) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.theme = theme
	a.dirty = true
	a.mu.Unlock()
	a.signal()
}

// Theme returns the App's configured theme, or the zero Theme if none was set.
func (a *App) Theme() Theme {
	if a == nil {
		return Theme{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.theme
}
