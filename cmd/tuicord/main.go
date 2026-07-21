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
	"awesomeProject/internal/uistate"

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

	cfg, startup, err := config.LoadStartup()
	if err != nil {
		return fmt.Errorf("prepare config: %w", err)
	}

	// One process owns config execution and the account session. In particular,
	// do not run config.lua twice in competing startup instances.
	releaseLock, err := acquireInstanceLock()
	if err != nil {
		return err
	}
	defer releaseLock()

	luaManager, pluginLog, err := newBootstrapPluginManager(&cfg)
	if err != nil {
		return fmt.Errorf("create Lua manager: %w", err)
	}
	var shell *ui.Shell
	defer func() {
		// Stop Lua while its attached Host targets are still alive.
		luaManager.Close()
		if shell != nil {
			shell.Close()
		}
		if pluginLog != nil {
			_ = pluginLog.Close()
		}
	}()
	if startup.ExecuteLua {
		if err := luaManager.LoadConfig(startup.LuaPath); err != nil {
			return fmt.Errorf("load config.lua: %w", err)
		}
	}
	if _, selected, ok := luaManager.ConsumeStartupTheme(); ok {
		config.ApplyTheme(&cfg, selected)
	}
	if cfg.ColorOverrides == nil {
		cfg.ColorOverrides = &config.ColorOverrides{Rules: map[string]config.ColorRule{}}
	}
	activeTheme, err := config.ThemeFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("resolve active theme: %w", err)
	}
	styles := uiStyles(cfg)

	state, err := uistate.Load()
	if err != nil {
		return fmt.Errorf("load UI state: %w", err)
	}
	if seedLegacyState(state, cfg) {
		if err := state.Save(); err != nil {
			return fmt.Errorf("seed UI state: %w", err)
		}
	}
	if *clearTokens {
		return clearStoredTokens(state)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	registry := state.AccountList()
	activeIdx := state.ActiveAccount()
	if len(registry) == 0 {
		registry = []uistate.Account{{Key: keyring.LegacyTokenKey}}
		activeIdx = 0
	}
	activeKey := registry[activeIdx].Key

	warnStore := func(err error) {
		fmt.Fprintln(os.Stderr, "tuicord: warning:", err)
	}
	loginPrompt := func(ctx context.Context) (string, error) {
		return ui.RunLogin(ctx, styles, tuiTheme(cfg.Colors, cfg.ColorOverrides), state.AuthPreferredMode, cfg.Accessibility, func(mode string) {
			state.AuthPreferredMode = mode
			if err := state.Save(); err != nil {
				fmt.Fprintln(os.Stderr, "tuicord: warning: save auth preference:", err)
			}
		})
	}

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
		tui.WithTheme(tuiTheme(cfg.Colors, cfg.ColorOverrides)),
		tui.WithMouse(cfg.Accessibility.MouseOn),
		tui.WithFocusableSplits(cfg.Accessibility.FocusSplits),
		tui.WithTTYColors(cfg.Display.TTYColors),
	)
	st := store.New(0)
	orch := app.New(ning, st, uiApp)
	mv := ui.NewMainViewWithState(orch, cfg, styles, state)
	shell = ui.NewShell(orch, mv, cfg, styles, stop)
	mv.OnPersistError(func(err error) { shell.ShowToast("View state", err) })

	surface := &uiSurface{mv: mv, shell: shell}
	seeds := make([]accounts.Seed, len(registry))
	for i, account := range registry {
		seeds[i] = accounts.Seed{Key: account.Key, Label: account.Label, ID: account.ID}
	}
	seeds[activeIdx].Ning = ning
	seeds[activeIdx].App = orch
	seeds[activeIdx].Store = st

	accountManager := accounts.New(accounts.Options{
		UI:      uiApp,
		Ctx:     ctx,
		Surface: surface,
		Build: func(key string) (*ningen.State, error) {
			token, err := keyring.GetTokenFor(key)
			if err != nil {
				return nil, fmt.Errorf("read token for account %q: %w", key, err)
			}
			token = strings.TrimSpace(token)
			if token == "" {
				return nil, fmt.Errorf("no saved token for account %q", key)
			}
			return discord.NewNingen(token)
		},
		Persist: func(reg config.Accounts) {
			state.Accounts = stateAccounts(reg)
			if err := state.Save(); err != nil {
				fmt.Fprintln(os.Stderr, "tuicord: warning: save accounts:", err)
			}
		},
		Seeds:       seeds,
		Active:      activeIdx,
		AutoConnect: true,
	})
	surface.manager = accountManager
	mv.SetAccountSelectHandler(func(i int) { _ = accountManager.Switch(i) })

	// Attach the live Host only after App/MainView/Shell exist. Config keymaps and
	// commands remain registered in the same manager; ordinary plugins now load
	// according to the Lua-derived Config.
	if errs := attachAndLoadPlugins(luaManager, orch, uiApp, shell, cfg, styles, activeTheme); len(errs) > 0 {
		shell.ShowToast("Plugin load", pluginLoadError(errs))
	}

	if err := accountManager.Start(); err != nil {
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

func seedLegacyState(state *uistate.State, cfg config.Config) bool {
	if state == nil {
		return false
	}
	changed := false
	if state.Accounts == nil && cfg.Accounts != nil {
		state.Accounts = stateAccounts(*cfg.Accounts)
		changed = true
	}
	if state.AuthPreferredMode == "" && cfg.Auth.PreferredMode != "" {
		state.AuthPreferredMode = cfg.Auth.PreferredMode
		changed = true
	}
	return changed
}

func stateAccounts(reg config.Accounts) *uistate.Accounts {
	out := &uistate.Accounts{Active: reg.Active, List: make([]uistate.Account, len(reg.List))}
	for i, account := range reg.List {
		out.List[i] = uistate.Account{Key: account.Key, Label: account.Label, ID: account.ID}
	}
	return out
}

// clearStoredTokens removes every machine-state registry token plus the legacy
// single-account key, so the next launch starts from interactive login.
func clearStoredTokens(state *uistate.State) error {
	keys := map[string]bool{keyring.LegacyTokenKey: true}
	for _, account := range state.AccountList() {
		if account.Key != "" {
			keys[account.Key] = true
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
		State: &ui.StyleState{},
	}
}

func tuiTheme(colors config.Colors, overrides *config.ColorOverrides) tui.Theme {
	cells := config.CellStyles(colors.Styles(), overrides)
	return tui.Theme{
		Background: cells["background"].Bg,
		Text:       cells["text"],
		Muted:      cells["muted"],
		Accent:     cells["accent"],
		Selection:  cells["selection"],
		Border:     cells["panels.border"],
		Error:      cells["error"],
	}
}

// theme is retained for focused wiring tests and older internal callers.
func theme(cfg config.Config) tui.Theme { return tuiTheme(cfg.Colors, cfg.ColorOverrides) }
