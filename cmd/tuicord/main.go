// Command tuicord is a terminal Discord client built on the internal/tui
// toolkit.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"awesomeProject/internal/accounts"
	"awesomeProject/internal/app"
	"awesomeProject/internal/auth"
	"awesomeProject/internal/config"
	"awesomeProject/internal/discord"
	"awesomeProject/internal/keyring"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/ui"

	"github.com/diamondburned/arikawa/v3/utils/ws"
	"github.com/diamondburned/ningen/v3"
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
	clearTokens := flag.Bool("clear", false, "remove saved account tokens from the OS keyring and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if *clearTokens {
		return clearStoredTokens(cfg)
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

	// Resolve which accounts exist and which is active. On first run (or a
	// pre-multi-account install) the single stored token is migrated into a
	// one-entry registry keyed by the legacy keyring key; its label is filled in
	// after the first connect.
	registry := cfg.AccountList()
	activeIdx := cfg.ActiveAccount()
	if len(registry) == 0 {
		registry = []config.Account{{Key: keyring.LegacyTokenKey}}
		activeIdx = 0
	}
	activeKey := registry[activeIdx].Key

	warnStore := func(err error) {
		// A missing Secret Service daemon must not prevent a valid pasted token
		// from being used for this session. The token is still read from TOKEN on
		// the next run, or can be pasted again.
		fmt.Fprintln(os.Stderr, "tuicord: warning:", err)
	}
	loginPrompt := func(ctx context.Context) (string, error) {
		return ui.RunLogin(ctx, styles, theme(cfg), cfg.Auth.PreferredMode, cfg.Accessibility, func(mode string) {
			cfg.Auth.PreferredMode = mode
			if err := config.Save(cfg); err != nil {
				fmt.Fprintln(os.Stderr, "tuicord: warning: save auth preference:", err)
			}
		})
	}

	// The active account's token is resolved — and interactively logged in if
	// missing — before the UI loop starts, since login takes over the terminal.
	token, err := auth.ResolveToken(ctx, auth.Options{
		Store:        auth.KeyringStore{Key: activeKey},
		OnStoreError: warnStore,
		Prompt:       loginPrompt,
	})
	if err != nil {
		return fmt.Errorf("resolve token: %w", err)
	}

	ning, err := discord.NewNingen(token)
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
	orch := app.New(ning, st, uiApp)

	mv := ui.NewMainView(orch, cfg, styles)
	shell := ui.NewShell(orch, mv, cfg, styles, stop)
	defer shell.Close()
	mv.OnPersistError(func(err error) {
		shell.ShowToast("View state", err)
	})

	// The account manager owns per-account callback routing (panel refreshes for
	// the active account, badges and notifications for every connected account),
	// lazy connect, and switching. The active account is handed in prebuilt (its
	// orchestrator/store back MainView/Shell); other accounts are built lazily
	// from the keyring on first switch, with no interactive prompt.
	surface := &uiSurface{mv: mv, shell: shell}
	seeds := make([]accounts.Seed, len(registry))
	for i, a := range registry {
		seeds[i] = accounts.Seed{Key: a.Key, Label: a.Label, ID: a.ID}
	}
	seeds[activeIdx].Ning = ning
	seeds[activeIdx].App = orch
	seeds[activeIdx].Store = st

	manager := accounts.New(accounts.Options{
		UI:      uiApp,
		Ctx:     ctx,
		Surface: surface,
		Build: func(key string) (*ningen.State, error) {
			t, err := keyring.GetTokenFor(key)
			if err != nil {
				return nil, fmt.Errorf("read token for account %q: %w", key, err)
			}
			// Stray whitespace in a stored token yields an opaque gateway 4004;
			// connect with the trimmed value.
			t = strings.TrimSpace(t)
			if t == "" {
				return nil, fmt.Errorf("no saved token for account %q", key)
			}
			return discord.NewNingen(t)
		},
		Persist: func(reg config.Accounts) {
			cfg.Accounts = &reg
			if err := config.Save(cfg); err != nil {
				fmt.Fprintln(os.Stderr, "tuicord: warning: save accounts:", err)
			}
		},
		Seeds:       seeds,
		Active:      activeIdx,
		AutoConnect: true,
	})
	surface.manager = manager
	mv.SetAccountSelectHandler(func(i int) { _ = manager.Switch(i) })

	// Plugins bind to the launch account's orchestrator; they do not follow
	// account switches in this phase.
	if mgr := setupPlugins(orch, uiApp, shell, cfg, styles); mgr != nil {
		// Registered after shell.Close above so LIFO shutdown stops/cancels all
		// plugin work while its Host targets are still alive.
		defer mgr.Close()
	}

	// Build, activate, and connect the active account (its LoadGuilds is
	// load-bearing; the user-session READY does not reliably deliver the guild
	// directory). Lazy connect means only visited accounts pull at launch.
	if err := manager.Start(); err != nil {
		return fmt.Errorf("start accounts: %w", err)
	}

	return uiApp.RunContext(ctx, shell)
}

// uiSurface adapts MainView + Shell to accounts.Surface. The accounts package
// stays free of a UI import; this adapter lives in main, which imports both.
type uiSurface struct {
	mv      *ui.MainView
	shell   *ui.Shell
	manager *accounts.Manager
}

func (u *uiSurface) Activate(a *accounts.Account) {
	u.mv.SetActiveAccount(a.App())
	u.shell.SetActiveAccount(a.App())
}

func (u *uiSurface) Refresh() { u.mv.Refresh() }

func (u *uiSurface) RefreshChannels() { u.mv.RefreshChannels() }

func (u *uiSurface) Notify(a *accounts.Account, msg store.Message) {
	if u.manager != nil && u.manager.Active() == a {
		u.shell.NotifyIncomingMessage(msg)
		return
	}
	u.shell.NotifyAccountMessage(a.Label(), msg)
}

func (u *uiSurface) ShowError(a *accounts.Account, err error) {
	// A 4004 close never recovers on retry: the stored token is dead. Remove it
	// so the next launch prompts for a fresh login instead of replaying it.
	if a != nil && a.Key() != "" && isGatewayAuthFailure(err) {
		if derr := keyring.DeleteTokenFor(a.Key()); derr != nil && !errors.Is(derr, keyring.ErrNotFound) {
			fmt.Fprintln(os.Stderr, "tuicord: warning: remove rejected token:", derr)
		}
	}
	title, toast := accountErrorToast(a, err)
	u.shell.ShowToast(title, toast)
}

func (u *uiSurface) Badges(bs []accounts.Badge) {
	rows := make([]ui.AccountBadge, len(bs))
	for i, b := range bs {
		rows[i] = ui.AccountBadge{Label: b.Label, Unread: b.Unread, Mentions: b.Mentions, Active: b.Active, Failed: b.Failed}
	}
	u.mv.SetAccounts(rows)
}

// accountErrorToast formats an account error for a toast, recognizing the
// gateway 4004 close (invalid/flagged token) with actionable text.
func accountErrorToast(a *accounts.Account, err error) (string, error) {
	title := "Discord error"
	if a != nil && a.Label() != "" {
		title = a.Label()
	}
	if isGatewayAuthFailure(err) {
		return "Authentication failed", errors.New("Discord rejected the session (close 4004). The token is invalid or the account was flagged — the saved token was removed; restart tuicord to log in again.")
	}
	return title, err
}

// isGatewayAuthFailure reports whether err is Discord's gateway 4004 close: a
// rejected IDENTIFY for an invalid or flagged token.
func isGatewayAuthFailure(err error) bool {
	var closeErr *ws.CloseEvent
	return errors.As(err, &closeErr) && closeErr.Code == gatewayAuthFailedCode
}

// clearStoredTokens removes every registry account's token — plus the legacy
// single-account token — from the OS keyring, so the next launch starts from
// an interactive login. Stale entries for accounts pruned from the registry by
// hand keep their config key here only while listed; the legacy key covers the
// pre-registry install.
func clearStoredTokens(cfg config.Config) error {
	keys := map[string]bool{keyring.LegacyTokenKey: true}
	for _, a := range cfg.AccountList() {
		if a.Key != "" {
			keys[a.Key] = true
		}
	}
	var failed []string
	for key := range keys {
		if err := keyring.DeleteTokenFor(key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			failed = append(failed, fmt.Sprintf("%s: %v", key, err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("clear tokens: %s", strings.Join(failed, "; "))
	}
	fmt.Println("tuicord: cleared stored tokens")
	return nil
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
