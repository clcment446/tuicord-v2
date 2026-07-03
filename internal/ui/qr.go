package ui

import (
	"context"

	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

// QRPanel renders the Discord remote-auth QR code and drives the login flow.
// The remote-auth protocol is implemented in qr_remoteauth.go; this widget owns
// only the on-screen state, which the flow updates via the App's post loop.
type QRPanel struct {
	app      *tui.App
	styles   Styles
	setToken func(string)
	status   string
	// matrix is the QR modules; nil until the fingerprint arrives.
	matrix [][]bool
	node   layout.Node
}

// NewQRPanel returns a QR panel and starts the remote-auth flow on a goroutine.
// Updates from that goroutine are marshaled onto the UI goroutine via app.Post.
func NewQRPanel(ctx context.Context, app *tui.App, styles Styles, setToken func(string)) *QRPanel {
	p := &QRPanel{
		app:      app,
		styles:   styles,
		setToken: setToken,
		status:   "Connecting…",
		node:     layout.Node{Grow: 1},
	}
	go runRemoteAuth(ctx, p)
	return p
}

// setStatus and setMatrix are called from the remote-auth flow (via update, so
// they run on the UI goroutine) to update the panel between redraws.
func (p *QRPanel) setStatus(s string)   { p.status = s }
func (p *QRPanel) setMatrix(m [][]bool) { p.matrix = m }

// update applies a mutation to the panel on the UI goroutine and redraws.
func (p *QRPanel) update(fn func()) {
	if p.app == nil {
		fn()
		return
	}
	p.app.Post(fn)
}

// Measure fills the available space.
func (p *QRPanel) Measure(avail tui.Size) tui.Size { return avail }

// Layout returns the layout node.
func (p *QRPanel) Layout() *layout.Node { return &p.node }

// Draw renders the QR code (two modules per character cell using half blocks)
// with the status line beneath it.
func (p *QRPanel) Draw(r screen.Region) {
	fill(r, p.styles.Text)
	if r.Width() <= 0 || r.Height() <= 0 {
		return
	}
	y := 0
	if p.matrix != nil {
		y = drawQR(r, p.matrix)
	}
	drawText(r, 0, min(y+1, r.Height()-1), p.status, p.styles.Muted)
}

// Handle ignores input; the panel is driven by the remote-auth flow.
func (p *QRPanel) Handle(tui.Event) bool { return false }

// drawQR renders the module matrix using the upper/lower half-block technique:
// each character row encodes two module rows via foreground (top) and
// background (bottom) colors. Returns the next free row.
func drawQR(r screen.Region, matrix [][]bool) int {
	on := screen.Style{Fg: screen.RGB(0, 0, 0), Bg: screen.RGB(255, 255, 255)}
	rows := len(matrix)
	y := 0
	for top := 0; top < rows; top += 2 {
		if y >= r.Height() {
			break
		}
		for x := 0; x < len(matrix[top]) && x < r.Width(); x++ {
			upper := matrix[top][x]
			lower := top+1 < rows && matrix[top+1][x]
			r.Set(x, y, halfBlock(upper, lower, on))
		}
		y++
	}
	return y
}

// halfBlock picks the glyph/colors so a light module is shown as the terminal's
// light color and a dark module as dark, matching a scannable QR.
func halfBlock(upper, lower bool, on screen.Style) screen.Cell {
	// Convention: true = dark module (drawn dark). We invert to keep quiet zones
	// light so phone cameras can lock on.
	switch {
	case upper && lower:
		return screen.Cell{Content: " ", Style: screen.Style{Bg: on.Fg}}
	case !upper && !lower:
		return screen.Cell{Content: " ", Style: screen.Style{Bg: on.Bg}}
	case upper && !lower:
		return screen.Cell{Content: "▀", Style: screen.Style{Fg: on.Fg, Bg: on.Bg}}
	default: // lower only
		return screen.Cell{Content: "▄", Style: screen.Style{Fg: on.Fg, Bg: on.Bg}}
	}
}
