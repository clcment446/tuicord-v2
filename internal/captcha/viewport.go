package captcha

// Viewport maps terminal-cell coordinates to browser viewport pixels.
// Coordinates are deliberately deterministic: the browser receives only the
// user's actual terminal events, with no synthetic movement or timing noise.
type Viewport struct {
	TerminalWidth  int
	TerminalHeight int
	BrowserWidth   int
	BrowserHeight  int
}

func (v Viewport) BrowserPoint(x, y int) (int, int, bool) {
	if v.TerminalWidth <= 0 || v.TerminalHeight <= 0 || v.BrowserWidth <= 0 || v.BrowserHeight <= 0 {
		return 0, 0, false
	}
	if x < 0 || y < 0 || x >= v.TerminalWidth || y >= v.TerminalHeight {
		return 0, 0, false
	}
	return x * v.BrowserWidth / v.TerminalWidth, y * v.BrowserHeight / v.TerminalHeight, true
}
