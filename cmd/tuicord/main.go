// Command tuicord is a terminal Discord client built on the internal/tui
// toolkit.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"awesomeProject/internal/app"
	"awesomeProject/internal/auth"
	"awesomeProject/internal/config"
	"awesomeProject/internal/discord"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tuicord:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	styles := uiStyles(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	token, err := auth.ResolveToken(ctx, auth.Options{
		Store: auth.KeyringStore{},
		Prompt: func(ctx context.Context) (string, error) {
			return ui.RunLogin(ctx, styles, theme(cfg))
		},
	})
	if err != nil {
		return fmt.Errorf("resolve token: %w", err)
	}

	sess, err := discord.NewSession(token)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	uiApp := tui.New(tui.WithTheme(theme(cfg)))
	st := store.New(0)
	orch := app.New(sess, st, uiApp)

	mv := ui.NewMainView(orch, cfg, styles)
	shell := ui.NewShell(orch, mv, cfg, styles, stop)
	mv.OnPersistError(func(err error) {
		shell.ShowToast("View state", err)
	})
	orch.OnReady(mv.Refresh)
	orch.OnChange(mv.RefreshChannels)
	orch.OnError(func(err error) {
		shell.ShowToast("Discord error", err)
	})
	orch.RegisterHandlers()
	orch.LoadGuilds(100)

	go func() {
		if err := orch.Connect(ctx); err != nil && ctx.Err() == nil {
			uiApp.Post(func() {
				shell.ShowToast("Gateway error", err)
			})
		}
	}()

	return uiApp.RunContext(ctx, shell)
}

// uiStyles resolves the configured colors into the palette the widgets draw with.
func uiStyles(cfg config.Config) ui.Styles {
	s := cfg.Colors.Styles()
	return ui.Styles{
		Text:    s.Text,
		Muted:   s.Muted,
		Accent:  s.Accent,
		Border:  s.Border,
		Pending: s.Muted,
		Error:   s.Error,
	}
}

// theme maps the configured palette onto the toolkit's Theme carrier.
func theme(cfg config.Config) tui.Theme {
	s := cfg.Colors.Styles()
	return tui.Theme{
		Background: s.Background,
		Text:       s.Text,
		Muted:      s.Muted,
		Accent:     s.Accent,
		Selection:  s.Selection,
		Border:     s.Border,
		Error:      s.Error,
	}
}
