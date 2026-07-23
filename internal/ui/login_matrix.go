package ui

import (
	"context"
	"strings"

	"awesomeProject/internal/auth"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// buildMatrixLogin composes the Matrix login panel: a homeserver, username, and
// password entry (submit on the password field), plus an access-token paste
// alternative. Network calls run off the UI goroutine; the status line reports
// progress and errors.
func buildMatrixLogin(ctx context.Context, app *tui.App, styles Styles, setMatrix func(auth.Credentials), authr MatrixAuthenticator) tui.Widget {
	homeserver := widget.NewTextInput("matrix.org")
	user := widget.NewTextInput("@user:matrix.org or localpart")
	password := widget.NewTextInput("password, press Enter")
	tokenInput := widget.NewTextInput("access token, press Enter")
	for _, in := range []*widget.TextInput{homeserver, user, password, tokenInput} {
		in.SetStyle(styles.Cell("login.input"))
		in.SetPlaceholderStyle(styles.Cell("login.placeholder"))
		in.SetCursorStyle(styles.Cell("login.cursor"))
	}
	homeserver.SetValue("matrix.org")

	status := widget.NewText("")
	status.SetStyle(styles.Cell("auth.status"))
	status.SetWrap(true)
	setStatus := func(msg string) {
		app.Post(func() {
			status.SetContent(msg)
			app.Invalidate()
		})
	}

	// busy guards against overlapping submissions. It is only ever read and
	// written on the UI goroutine: submit handlers run there, and the login
	// goroutines report failure by posting the reset back onto it (via failed).
	var busy bool
	failed := func(msg string) {
		app.Post(func() {
			busy = false
			status.SetContent(msg)
			app.Invalidate()
		})
	}
	submitPassword := func(string) {
		if busy {
			return
		}
		hs := strings.TrimSpace(homeserver.Value())
		u := strings.TrimSpace(user.Value())
		p := password.Value()
		if hs == "" || u == "" || p == "" {
			setStatus("Enter homeserver, username, and password.")
			return
		}
		busy = true
		setStatus("Logging in…")
		go func() {
			creds, err := authr.Password(ctx, hs, u, p)
			if err != nil {
				failed("Login failed: " + err.Error())
				return
			}
			setMatrix(creds)
		}()
	}
	password.OnSubmit(submitPassword)

	submitToken := func(string) {
		if busy {
			return
		}
		hs := strings.TrimSpace(homeserver.Value())
		tok := strings.TrimSpace(tokenInput.Value())
		if hs == "" || tok == "" {
			setStatus("Enter homeserver and access token.")
			return
		}
		busy = true
		setStatus("Validating token…")
		go func() {
			creds, err := authr.Token(ctx, hs, tok)
			if err != nil {
				failed("Token invalid: " + err.Error())
				return
			}
			setMatrix(creds)
		}()
	}
	tokenInput.OnSubmit(submitToken)

	return widget.Column(
		loginLabel(styles, "Log in to Matrix"),
		loginLabel(styles, ""),
		titled(styles, "Homeserver", homeserver),
		titled(styles, "Username", user),
		titled(styles, "Password", password),
		loginLabel(styles, ""),
		loginLabel(styles, "Or paste an existing access token:"),
		titled(styles, "Access token", tokenInput),
		loginLabel(styles, ""),
		status,
	)
}
