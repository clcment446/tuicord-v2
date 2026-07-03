package tui

import "awesomeProject/internal/tui/screen"

// Theme is a small palette of styles the application applies to its widgets.
//
// The runtime does not draw with the theme itself; widgets are styled
// explicitly by the caller. Carrying the theme on the App gives a single place
// for configuration (loaded from disk, say) to reach the code that builds the
// widget tree.
type Theme struct {
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

// Theme returns the App's configured theme, or the zero Theme if none was set.
func (a *App) Theme() Theme {
	if a == nil {
		return Theme{}
	}
	return a.theme
}
