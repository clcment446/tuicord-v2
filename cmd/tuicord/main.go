// Command tuicord is a terminal Discord client built on the internal/tui
// toolkit.
package main

import (
	"context"
	"errors"
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

	"github.com/diamondburned/arikawa/v3/utils/ws"
)

// gatewayAuthFailedCode is Discord's gateway close code for a rejected
// IDENTIFY: the token is invalid or the account has been flagged.
const gatewayAuthFailedCode = 4004

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

	// One instance per account. Two clients on one token double the gateway
	// sessions and startup REST volume, which is what trips Discord's abuse
	// detection.
	releaseLock, err := acquireInstanceLock()
	if err != nil {
		return err
	}
	defer releaseLock()
	// Ensure a non-nil override set so plugins can add color rules at runtime.
	// The Styles built below and the plugin Host share this same pointer, so a
	// tuicord.style call is visible to widgets on the next render.
	if cfg.ColorOverrides == nil {
		cfg.ColorOverrides = &config.ColorOverrides{Rules: map[string]config.ColorRule{}}
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
	defer shell.Close()
	mv.OnPersistError(func(err error) {
		shell.ShowToast("View state", err)
	})
	orch.OnReady(mv.Refresh)
	orch.OnGuildChange(mv.Refresh)
	orch.OnChange(mv.RefreshChannels)
	orch.OnIncomingMessage(shell.NotifyIncomingMessage)
	orch.OnError(func(err error) {
		shell.ShowToast("Discord error", err)
	})
	if mgr := setupPlugins(orch, uiApp, shell, cfg, styles); mgr != nil {
		// Registered after shell.Close above so LIFO shutdown stops/cancels all
		// plugin work while its Host targets are still alive.
		defer mgr.Close()
	}

	orch.RegisterHandlers()
	// The user-session gateway READY does not reliably deliver the guild/DM
	// directory, so this REST pull is load-bearing, not redundant. Its DM
	// hydration is bounded (see hydratePrivateChannels) to avoid a launch burst.
	orch.LoadGuilds(100)

	go func() {
		err := orch.Connect(ctx)
		if err == nil || ctx.Err() != nil {
			return
		}
		title, toast := "Gateway error", err
		var closeErr *ws.CloseEvent
		if errors.As(err, &closeErr) && closeErr.Code == gatewayAuthFailedCode {
			title = "Authentication failed"
			toast = errors.New("Discord rejected the session (close 4004). The token is invalid or the account was flagged — re-authenticate to continue.")
		}
		uiApp.Post(func() {
			shell.ShowToast(title, toast)
		})
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
