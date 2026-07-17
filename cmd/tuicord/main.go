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
		OnStoreError: func(err error) {
			// A missing Secret Service daemon must not prevent a valid pasted
			// token from being used for this session. The token is still read
			// from TOKEN on the next run, or can be pasted again.
			fmt.Fprintln(os.Stderr, "tuicord: warning:", err)
		},
		Prompt: func(ctx context.Context) (string, error) {
			return ui.RunLogin(ctx, styles, theme(cfg), cfg.Auth.PreferredMode, cfg.Accessibility, func(mode string) {
				cfg.Auth.PreferredMode = mode
				if err := config.Save(cfg); err != nil {
					fmt.Fprintln(os.Stderr, "tuicord: warning: save auth preference:", err)
				}
			})
		},
	})
	if err != nil {
		return fmt.Errorf("resolve token: %w", err)
	}

	sess, err := discord.NewSession(token)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	uiApp := tui.New(
		tui.WithTheme(theme(cfg)),
		tui.WithMouse(cfg.Accessibility.MouseOn),
		tui.WithFocusableSplits(cfg.Accessibility.FocusSplits),
		tui.WithTTYColors(cfg.Display.TTYColors),
	)
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
	cells := config.CellStyles(s, cfg.ColorOverrides)
	custom := config.CustomCellKeys(cells, cfg.ColorOverrides)
	return ui.Styles{
		Text:    cells["messages.content"],
		Muted:   cells["muted"],
		Accent:  cells["accent"],
		Border:  cells["panels.border"],
		Pending: cells["pending"],
		Error:   cells["error"],
		Cells:   cells, Custom: custom, Overrides: cfg.ColorOverrides,
	}
}

// theme maps the configured palette onto the toolkit's Theme carrier.
func theme(cfg config.Config) tui.Theme {
	s := cfg.Colors.Styles()
	cells := config.CellStyles(s, cfg.ColorOverrides)
	return tui.Theme{
		Background: cells["background"].Bg,
		Text:       cells["text"],
		Muted:      cells["muted"],
		Accent:     cells["accent"],
		Selection:  cells["guilds.selected"],
		Border:     cells["panels.border"],
		Error:      cells["error"],
	}
}
