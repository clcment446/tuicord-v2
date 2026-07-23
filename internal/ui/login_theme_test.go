package ui

import (
	"context"
	"testing"

	"awesomeProject/internal/auth"
	"awesomeProject/internal/config"
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

type stubMatrixAuth struct{}

func (stubMatrixAuth) Password(context.Context, string, string, string) (auth.Credentials, error) {
	return auth.Credentials{}, nil
}
func (stubMatrixAuth) Token(context.Context, string, string) (auth.Credentials, error) {
	return auth.Credentials{}, nil
}

func mochaStyles(t *testing.T) (Styles, tui.Theme) {
	t.Helper()
	cfg := config.Default()
	cfg.Colors.Enabled = true
	cfg.Colors.Background = "#1e1e2e"
	cfg.Colors.Text = "#cdd6f4"
	cfg.Colors.Muted = "#a6adc8"
	cfg.Colors.Accent = "#89b4fa"
	cfg.Colors.Selection = "#313244"
	cfg.Colors.Border = "#45475a"
	cfg.Colors.Error = "#f38ba8"
	cfg.ColorOverrides = &config.ColorOverrides{Rules: map[string]config.ColorRule{}}
	cells := config.CellStyles(cfg.Colors.Styles(), cfg.ColorOverrides)
	styles := Styles{
		Text: cells["messages.content"], Muted: cells["muted"], Accent: cells["accent"],
		Border: cells["panels.border"], Pending: cells["pending"], Error: cells["error"],
		Cells: cells, Custom: map[string]bool{}, Overrides: cfg.ColorOverrides,
		State: &StyleState{},
	}
	theme := tui.Theme{
		Background: cells["background"].Bg,
		Text:       cells["text"],
		Accent:     cells["accent"],
		Border:     cells["panels.border"],
	}
	return styles, theme
}

// TestLoginSurfaceFullyThemed guards against the login screen punching
// terminal-default holes through the theme background. Unstyled widget.NewText
// labels and unstyled Split dividers clear their region with a background-less
// style, so on any terminal whose default background differs from the theme the
// login screen renders as a patchwork. Every drawn cell must carry the theme
// background except the text-input cursor cells, which are intentionally
// terminal-default (login.cursor has no background so the terminal draws its
// own cursor there).
func TestLoginSurfaceFullyThemed(t *testing.T) {
	styles, theme := mochaStyles(t)
	app := tui.New(tui.WithTheme(theme))

	root := buildLogin(context.Background(), app, styles,
		func(string) {}, func(auth.Credentials) {}, func() {}, "", func(string) {}, stubMatrixAuth{})

	const W, H = 100, 44
	buf := screen.NewBuffer(W, H)
	buf.Fill(screen.Rect{W: W, H: H}, screen.Cell{Content: " ", Style: screen.Style{Bg: theme.Background}})
	hits := tui.BuildHitIndex(root, tui.Size{W: W, H: H})
	for _, e := range hits.Entries() {
		e.Widget.Draw(buf.ClipWithin(
			screen.Rect{X: e.Rect.X, Y: e.Rect.Y, W: e.Rect.W, H: e.Rect.H},
			screen.Rect{X: e.Clip.X, Y: e.Clip.Y, W: e.Clip.W, H: e.Clip.H},
		))
	}

	// Allow a small budget for input cursor cells (login.cursor is intentionally
	// background-less). Anything beyond that is an unthemed hole regression.
	const cursorBudget = 6
	holes := 0
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if !buf.Cell(x, y).Style.Bg.Set() {
				holes++
			}
		}
	}
	if holes > cursorBudget {
		t.Fatalf("login screen has %d unthemed (unset-background) cells; want <= %d", holes, cursorBudget)
	}
}

// TestSignInOverlayThemedAndReports renders the `;signin` overlay tree and
// checks both that it is themed (same hole guard as the login screen) and that
// a pasted Discord token flows through to the setToken callback.
func TestSignInOverlayThemedAndReports(t *testing.T) {
	styles, theme := mochaStyles(t)
	app := tui.New(tui.WithTheme(theme))

	var gotToken string
	root := buildSignIn(context.Background(), app, styles,
		func(tok string) { gotToken = tok }, func(auth.Credentials) {}, func() {}, stubMatrixAuth{})

	const W, H = 100, 30
	buf := screen.NewBuffer(W, H)
	buf.Fill(screen.Rect{W: W, H: H}, screen.Cell{Content: " ", Style: screen.Style{Bg: theme.Background}})
	hits := tui.BuildHitIndex(root, tui.Size{W: W, H: H})
	for _, e := range hits.Entries() {
		e.Widget.Draw(buf.ClipWithin(
			screen.Rect{X: e.Rect.X, Y: e.Rect.Y, W: e.Rect.W, H: e.Rect.H},
			screen.Rect{X: e.Clip.X, Y: e.Clip.Y, W: e.Clip.W, H: e.Clip.H},
		))
	}
	holes := 0
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if !buf.Cell(x, y).Style.Bg.Set() {
				holes++
			}
		}
	}
	const cursorBudget = 6
	if holes > cursorBudget {
		t.Fatalf("sign-in overlay has %d unthemed cells; want <= %d", holes, cursorBudget)
	}

	// The Discord token field submits through setToken.
	ti := findTokenInput(root)
	if ti == nil {
		t.Fatal("no token input found in sign-in overlay")
	}
	ti.SetValue("abc.def.ghi")
	ti.Handle(input.KeyEvent{Key: input.KeyEnter})
	if gotToken != "abc.def.ghi" {
		t.Fatalf("setToken got %q, want %q", gotToken, "abc.def.ghi")
	}
}

// findTokenInput walks the retained tree for the first text input (the Discord
// token field, which is placed first in the sign-in overlay).
func findTokenInput(w tui.Widget) *widget.TextInput {
	if ti, ok := w.(*widget.TextInput); ok {
		return ti
	}
	container, ok := w.(tui.Container)
	if !ok {
		return nil
	}
	for _, c := range container.Children() {
		if c == nil {
			continue
		}
		if ti := findTokenInput(c); ti != nil {
			return ti
		}
	}
	return nil
}
