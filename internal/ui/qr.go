package ui

import (
	"context"
	"image"

	"awesomeProject/internal/captcha"
	"awesomeProject/internal/config"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// QRPanel renders the Discord remote-auth QR code and drives the login flow.
// The remote-auth protocol is implemented in qr_remoteauth.go; this widget owns
// only the on-screen state, which the flow updates via the App's post loop.
type QRPanel struct {
	app             *tui.App
	styles          Styles
	setToken        func(string)
	status          string
	captchaStatus   string
	captchaImage    *widget.Image
	captchaViewport captcha.Viewport
	captchaSession  *captcha.Session
	preferredMode   string
	onModeSelected  func(string)
	modePrompt      bool
	modeSelection   int
	modeResult      chan string
	// matrix is the QR modules; nil until the fingerprint arrives.
	matrix [][]bool
	node   layout.Node
}

// NewQRPanel returns a QR panel and starts the remote-auth flow on a goroutine.
// Updates from that goroutine are marshaled onto the UI goroutine via app.Post.
func NewQRPanel(ctx context.Context, app *tui.App, styles Styles, setToken func(string), preferredMode string, onModeSelected func(string)) *QRPanel {
	p := &QRPanel{
		app:            app,
		styles:         styles,
		setToken:       setToken,
		status:         "Connecting…",
		preferredMode:  preferredMode,
		onModeSelected: onModeSelected,
		node:           layout.Node{Grow: 1},
	}
	go runRemoteAuth(ctx, p, preferredMode)
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
	fill(r, p.styles.Cell("auth.qr"))
	if r.Width() <= 0 || r.Height() <= 0 {
		return
	}
	if p.modePrompt {
		fill(r, p.styles.Cell("auth.qr"))
		drawText(r, 1, 1, "CAPTCHA required after QR approval", p.styles.Cell("auth.title"))
		drawText(r, 1, 3, "Choose how to complete it:", p.styles.Cell("auth.qr"))
		first, second := modeChoices(p.preferredMode)
		drawModeChoice(r, 1, 5, loginModeLabel(first), p.modeSelection == 0, first == config.AuthModeTUI, p.styles)
		drawModeChoice(r, 1, 6, loginModeLabel(second), p.modeSelection == 1, second == config.AuthModeTUI, p.styles)
		drawText(r, 1, min(8, r.Height()-1), "Enter select • ↑/↓ choose • Esc cancel", p.styles.Cell("auth.hint"))
		return
	}
	if p.captchaImage != nil {
		imageHeight := max(1, r.Height()-1)
		p.captchaViewport = captcha.Viewport{
			TerminalWidth: r.Width(), TerminalHeight: imageHeight,
			BrowserWidth: p.captchaImageWidth(), BrowserHeight: p.captchaImageHeight(),
		}
		p.captchaImage.Draw(r.Clip(screen.Rect{W: r.Width(), H: imageHeight}))
		drawText(r, 0, r.Height()-1, p.captchaStatus, p.styles.Cell("auth.status"))
		return
	}
	y := 0
	if p.matrix != nil {
		var ok bool
		y, ok = drawQRStyled(r, p.matrix, p.styles.Cell("auth.qr.dark"), p.styles.Cell("auth.qr.light"))
		if !ok {
			drawText(r, 0, 0, "Terminal too small for QR code.", p.styles.Cell("auth.hint"))
			drawText(r, 0, min(1, r.Height()-1), "Make this pane wider or taller.", p.styles.Cell("auth.hint"))
			return
		}
	}
	drawText(r, 0, min(y+1, r.Height()-1), p.status, p.styles.Cell("auth.status"))
}

// Handle ignores input; the panel is driven by the remote-auth flow.
func (p *QRPanel) Handle(ev tui.Event) bool {
	if p.modePrompt {
		return p.handleModePrompt(ev)
	}
	if p.captchaSession == nil {
		return false
	}
	switch ev := ev.(type) {
	case input.MouseEvent:
		x, y, ok := p.captchaViewport.BrowserPoint(ev.X, ev.Y)
		if !ok {
			return true
		}
		actions := browserMouseActions(ev, x, y)
		if len(actions) == 0 {
			return true
		}
		go p.captchaSession.PerformActions(context.Background(), []map[string]any{{
			"type": "pointer", "id": "tuicord-mouse", "parameters": map[string]any{"pointerType": "mouse"}, "actions": actions,
		}})
		return true
	case input.KeyEvent:
		value, ok := browserKey(ev)
		if !ok {
			return false
		}
		typeName := "keyDown"
		if ev.Release {
			typeName = "keyUp"
		}
		go p.captchaSession.PerformActions(context.Background(), []map[string]any{{
			"type": "key", "id": "tuicord-keyboard", "actions": []map[string]any{{"type": typeName, "value": value}},
		}})
		return true
	}
	return false
}

func (p *QRPanel) waitForCaptchaMode(ctx context.Context) (string, error) {
	if p.app == nil {
		return normalizedLoginMode(p.preferredMode), nil
	}
	result := make(chan string, 1)
	p.update(func() {
		p.modeSelection = 0
		p.modePrompt = true
		p.modeResult = result
		p.status = "CAPTCHA required. Choose a verification method."
	})
	select {
	case mode := <-result:
		if mode == "" {
			return "", ErrLoginAborted
		}
		if p.onModeSelected != nil {
			p.onModeSelected(mode)
		}
		return mode, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (p *QRPanel) handleModePrompt(ev tui.Event) bool {
	selectMode := func(mode string) {
		if p.modeResult == nil {
			return
		}
		p.modePrompt = false
		result := p.modeResult
		p.modeResult = nil
		result <- mode
	}
	switch ev := ev.(type) {
	case input.KeyEvent:
		if ev.Release {
			return true
		}
		switch ev.Key {
		case input.KeyUp, input.KeyLeft:
			p.modeSelection = 0
		case input.KeyDown, input.KeyRight:
			p.modeSelection = 1
		case input.KeyEnter:
			first, second := modeChoices(p.preferredMode)
			if p.modeSelection == 0 {
				selectMode(first)
			} else {
				selectMode(second)
			}
		case input.KeyEsc:
			if p.modeResult != nil {
				result := p.modeResult
				p.modeResult = nil
				p.modePrompt = false
				close(result)
			}
		}
		return true
	case input.MouseEvent:
		if ev.Kind != input.MousePress {
			return true
		}
		if ev.Y == 5 {
			first, _ := modeChoices(p.preferredMode)
			selectMode(first)
		} else if ev.Y == 6 {
			_, second := modeChoices(p.preferredMode)
			selectMode(second)
		}
		return true
	}
	return true
}

func modeChoices(preferred string) (string, string) {
	if normalizedLoginMode(preferred) == config.AuthModeBrowser {
		return config.AuthModeBrowser, config.AuthModeTUI
	}
	return config.AuthModeTUI, config.AuthModeBrowser
}

func drawModeChoice(r screen.Region, x, y int, label string, selected, preferred bool, styles Styles) {
	marker := "  "
	if selected {
		marker = "> "
	}
	hint := ""
	if preferred {
		hint = " (preferred)"
	}
	drawText(r, x, y, marker+label+hint, styles.Cell("auth.choice"))
}

func browserMouseActions(ev input.MouseEvent, x, y int) []map[string]any {
	move := map[string]any{"type": "pointerMove", "x": x, "y": y, "duration": 0, "origin": "viewport"}
	switch ev.Kind {
	case input.MouseMotion:
		return []map[string]any{move}
	case input.MousePress:
		return []map[string]any{move, {"type": "pointerDown", "button": mouseButton(ev.Btn)}}
	case input.MouseRelease:
		return []map[string]any{move, {"type": "pointerUp", "button": mouseButton(ev.Btn)}}
	default:
		return nil
	}
}

func (p *QRPanel) setCaptchaSession(session *captcha.Session, img image.Image) {
	p.captchaSession = session
	p.captchaImage = widget.NewImageFrom(img).SetMode(widget.ImageKitty).SetID(77)
	p.captchaStatus = "Complete the CAPTCHA in the terminal."
}

func (p *QRPanel) setCaptchaFrame(img image.Image) {
	if p.captchaImage == nil {
		p.captchaImage = widget.NewImageFrom(img).SetMode(widget.ImageKitty).SetID(77)
		return
	}
	p.captchaImage.SetImage(img)
}

func (p *QRPanel) captchaImageWidth() int {
	if p.captchaImage == nil {
		return 1
	}
	return p.captchaImageWidthFromMeasure().W
}

func (p *QRPanel) captchaImageHeight() int {
	if p.captchaImage == nil {
		return 1
	}
	return p.captchaImageWidthFromMeasure().H * 2
}

func (p *QRPanel) captchaImageWidthFromMeasure() tui.Size {
	if p.captchaImage == nil {
		return tui.Size{W: 1, H: 1}
	}
	return p.captchaImage.Measure(tui.Size{})
}

func mouseButton(btn input.Button) int {
	switch btn {
	case input.ButtonMiddle:
		return 1
	case input.ButtonRight:
		return 2
	default:
		return 0
	}
}

func browserKey(ev input.KeyEvent) (string, bool) {
	if ev.Key == input.KeyRune {
		return string(ev.Rune), true
	}
	keys := map[input.Key]string{
		input.KeyEnter: "\ue007", input.KeyTab: "\ue004", input.KeyBackspace: "\ue003", input.KeyEsc: "\ue00c",
	}
	value, ok := keys[ev.Key]
	return value, ok
}

// drawQR renders the module matrix using the upper/lower half-block technique:
// each character row encodes two module rows. Returns the next free row and
// whether the whole code fit; clipped QR codes are usually unscannable.
func drawQR(r screen.Region, matrix [][]bool) (int, bool) {
	return drawQRStyled(r, matrix, screen.Style{Fg: screen.RGB(0, 0, 0), Bg: screen.RGB(255, 255, 255)}, screen.Style{Fg: screen.RGB(255, 255, 255), Bg: screen.RGB(0, 0, 0)})
}

func drawQRStyled(r screen.Region, matrix [][]bool, dark, light screen.Style) (int, bool) {
	rows := len(matrix)
	cols := qrCols(matrix)
	qrRows := (rows + 1) / 2
	if rows == 0 || cols == 0 {
		return 0, true
	}
	if cols > r.Width() || qrRows > r.Height() {
		return 0, false
	}

	x0 := (r.Width() - cols) / 2
	y := 0
	for top := 0; top < rows; top += 2 {
		for x := 0; x < cols; x++ {
			upper := qrModule(matrix, top, x)
			lower := qrModule(matrix, top+1, x)
			r.Set(x0+x, y, halfBlockStyled(upper, lower, dark, light))
		}
		y++
	}
	return y, true
}

func qrCols(matrix [][]bool) int {
	cols := 0
	for _, row := range matrix {
		if len(row) > cols {
			cols = len(row)
		}
	}
	return cols
}

func qrModule(matrix [][]bool, row, col int) bool {
	return row >= 0 && row < len(matrix) && col >= 0 && col < len(matrix[row]) && matrix[row][col]
}

// halfBlock picks the glyph/colors so a light module is shown as the terminal's
// light color and a dark module as dark, matching a scannable QR.
func halfBlock(upper, lower bool, on screen.Style) screen.Cell {
	return halfBlockStyled(upper, lower, on, screen.Style{Fg: on.Bg, Bg: on.Fg})
}

func halfBlockStyled(upper, lower bool, dark, light screen.Style) screen.Cell {
	// Convention: true = dark module (drawn dark). We invert to keep quiet zones
	// light so phone cameras can lock on.
	switch {
	case upper && lower:
		return screen.Cell{Content: " ", Style: screen.Style{Bg: dark.Fg}}
	case !upper && !lower:
		return screen.Cell{Content: " ", Style: screen.Style{Bg: light.Bg}}
	case upper && !lower:
		return screen.Cell{Content: "▀", Style: screen.Style{Fg: dark.Fg, Bg: light.Bg}}
	default: // lower only
		return screen.Cell{Content: "▄", Style: screen.Style{Fg: dark.Fg, Bg: light.Bg}}
	}
}
