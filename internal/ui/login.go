package ui

import (
	"context"
	"errors"
	"strings"

	"awesomeProject/internal/auth"
	"awesomeProject/internal/config"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// ErrLoginAborted is returned when the login screen closes without a token.
var ErrLoginAborted = errors.New("login aborted")

// LoginResult is the outcome of the login screen. Exactly one of the Discord
// token or the Matrix credentials is set. KeyringValue returns the string to
// persist under the account's keyring key.
type LoginResult struct {
	token  string
	matrix *auth.Credentials
}

// KeyringValue returns the value to store in the keyring: a bare Discord token,
// or the encoded Matrix credentials blob.
func (r LoginResult) KeyringValue() (string, error) {
	if r.matrix != nil {
		return r.matrix.Encode()
	}
	return r.token, nil
}

// MatrixAuthenticator performs the network side of Matrix login. It is injected
// by the caller (cmd) so this package needs no Matrix/mautrix dependency.
type MatrixAuthenticator interface {
	// Password resolves homeserver via .well-known and logs in with a password.
	Password(ctx context.Context, homeserver, user, password string) (auth.Credentials, error)
	// Token validates an existing access token against a homeserver.
	Token(ctx context.Context, homeserver, token string) (auth.Credentials, error)
}

// RunLogin shows the interactive login screen and blocks until the user
// provides Discord or Matrix credentials, or aborts. It runs its own tui
// runtime, separate from the main app. matrixAuth may be nil to disable the
// Matrix panel.
func RunLogin(ctx context.Context, styles Styles, theme tui.Theme, preferredMode string, accessibility config.Accessibility, onModeSelected func(string), matrixAuth MatrixAuthenticator) (LoginResult, error) {
	loginCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	app := tui.New(
		tui.WithTheme(theme),
		tui.WithMouse(accessibility.MouseOn),
		tui.WithFocusableSplits(accessibility.FocusSplits),
	)

	var result LoginResult
	setToken := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		result = LoginResult{token: t}
		cancel()
	}
	setMatrix := func(c auth.Credentials) {
		creds := c
		result = LoginResult{matrix: &creds}
		cancel()
	}

	root := buildLogin(loginCtx, app, styles, setToken, setMatrix, cancel, preferredMode, onModeSelected, matrixAuth)
	if err := app.RunContext(loginCtx, root); err != nil {
		return LoginResult{}, err
	}
	if result.token == "" && result.matrix == nil {
		return LoginResult{}, ErrLoginAborted
	}
	return result, nil
}

// buildLogin composes the login layout: Discord (token + QR) stacked over the
// Matrix panel.
func buildLogin(ctx context.Context, app *tui.App, styles Styles, setToken func(string), setMatrix func(auth.Credentials), cancel context.CancelFunc, preferredMode string, onModeSelected func(string), matrixAuth MatrixAuthenticator) tui.Widget {
	tokenInput := widget.NewTextInput("Paste token, press Enter")
	tokenInput.SetStyle(styles.Cell("login.input"))
	tokenInput.SetPlaceholderStyle(styles.Cell("login.placeholder"))
	tokenInput.SetCursorStyle(styles.Cell("login.cursor"))
	tokenInput.OnSubmit(setToken)

	tokenPanel := widget.Column(
		widget.NewText("Log in to Discord"),
		widget.NewText(""),
		widget.NewText("Option 1 — paste a token:"),
		titled(styles, "Token", tokenInput),
		widget.NewText(""),
		widget.NewText("Option 2 — scan the QR code with the Discord mobile app."),
	)

	qr := NewQRPanel(ctx, app, styles, setToken, preferredMode, onModeSelected)

	discord := widget.NewSplit(titled(styles, "Discord", tokenPanel), titled(styles, "QR Code", qr)).
		Basis(36).
		MinFirst(30).
		Vertical()
	discord.SetBorderChars(styles.BorderCharsOrDefault())

	if matrixAuth == nil {
		return newCancelRoot(discord, cancel)
	}

	matrixPanel := buildMatrixLogin(ctx, app, styles, setMatrix, matrixAuth)
	root := widget.NewSplit(discord, titled(styles, "Matrix", matrixPanel)).
		Basis(0).
		Horizontal()
	root.SetBorderChars(styles.BorderCharsOrDefault())
	return newCancelRoot(root, cancel)
}

func normalizedLoginMode(mode string) string {
	if mode == config.AuthModeBrowser {
		return config.AuthModeBrowser
	}
	return config.AuthModeTUI
}

func loginModeLabel(mode string) string {
	if mode == config.AuthModeBrowser {
		return "Open a full Firefox window"
	}
	return "Solve CAPTCHA inside the terminal"
}

type cancelRoot struct {
	child  tui.Widget
	cancel context.CancelFunc
	node   layout.Node
}

func newCancelRoot(child tui.Widget, cancel context.CancelFunc) *cancelRoot {
	r := &cancelRoot{child: child, cancel: cancel, node: layout.Node{Grow: 1}}
	if child != nil {
		r.node.Children = []*layout.Node{child.Layout()}
	}
	return r
}

func (r *cancelRoot) Children() []tui.Widget {
	if r == nil || r.child == nil {
		return nil
	}
	return []tui.Widget{r.child}
}

func (r *cancelRoot) Measure(avail tui.Size) tui.Size {
	if r == nil || r.child == nil {
		return avail
	}
	return r.child.Measure(avail)
}

func (r *cancelRoot) Layout() *layout.Node {
	if r == nil {
		return nil
	}
	return &r.node
}

func (r *cancelRoot) Draw(screen.Region) {}

func (r *cancelRoot) Handle(ev tui.Event) bool {
	key, ok := ev.(input.KeyEvent)
	if ok && !key.Release && key.Key == input.KeyRune && key.Rune == 'c' && key.Mods&input.Ctrl != 0 {
		if r.cancel != nil {
			r.cancel()
		}
		return true
	}
	return r != nil && r.child != nil && r.child.Handle(ev)
}
