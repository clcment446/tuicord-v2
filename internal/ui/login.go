package ui

import (
	"context"
	"errors"
	"strings"

	"awesomeProject/internal/config"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// ErrLoginAborted is returned when the login screen closes without a token.
var ErrLoginAborted = errors.New("login aborted")

// RunLogin shows the interactive login screen and blocks until the user
// provides a token — either by pasting one or by completing the QR remote-auth
// flow — or aborts. It runs its own tui runtime, separate from the main app.
//
// It satisfies auth.PromptFunc when wrapped: the returned token is persisted by
// auth.ResolveToken.
func RunLogin(ctx context.Context, styles Styles, theme tui.Theme, preferredMode string, accessibility config.Accessibility, onModeSelected func(string)) (string, error) {
	loginCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	app := tui.New(
		tui.WithTheme(theme),
		tui.WithMouse(accessibility.MouseOn),
		tui.WithFocusableSplits(accessibility.FocusSplits),
	)

	var token string
	setToken := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		token = t
		cancel()
	}

	root := buildLogin(loginCtx, app, styles, setToken, cancel, preferredMode, onModeSelected)
	if err := app.RunContext(loginCtx, root); err != nil {
		return "", err
	}
	if token == "" {
		return "", ErrLoginAborted
	}
	return token, nil
}

// buildLogin composes the login layout: a token entry on the left and the QR
// remote-auth panel on the right.
func buildLogin(ctx context.Context, app *tui.App, styles Styles, setToken func(string), cancel context.CancelFunc, preferredMode string, onModeSelected func(string)) tui.Widget {
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

	root := widget.NewSplit(titled(styles, "Login", tokenPanel), titled(styles, "QR Code", qr)).
		Basis(36).
		MinFirst(30).
		Vertical()
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
