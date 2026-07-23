// Package accounts manages multiple chat accounts within a single tuicord
// process. Each account owns its own protocol backend (backend.Backend, e.g.
// the Discord orchestrator app.App) and normalized store; all of them post onto
// the one shared tui runtime, so the single-UI-goroutine concurrency model is
// unchanged. The package depends only on backend.Backend, never a concrete
// protocol library. The Manager tracks
// which account is active, connects accounts lazily (only on first activation,
// then keeps them connected), routes each account's callbacks — panel refreshes
// only for the active account, badges and notifications for every connected
// account — and drives the UI through the Surface interface so this package
// never imports internal/ui.
package accounts

import (
	"context"
	"errors"
	"fmt"

	"awesomeProject/internal/backend"
	"awesomeProject/internal/config"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"
)

// ErrNoAccounts is returned by Start when the registry is empty.
var ErrNoAccounts = errors.New("accounts: no accounts configured")

// Surface is the view the Manager drives. It is implemented by a small adapter
// over the MainView/Shell/selector so this package stays free of a UI import
// (and remains unit-testable with a fake). Every method runs on the UI
// goroutine.
type Surface interface {
	// Activate rebinds the visible panels to the given account's orchestrator
	// and store and performs a full refresh. Called on switch.
	Activate(a *Account)
	// Refresh redraws the guild/channel panels for the active account (used on
	// its ready/guild changes, after Activate has bound it).
	Refresh()
	// RefreshChannels redraws the channel panel for the active account.
	RefreshChannels()
	// RefreshGuildBadges redraws guild/channel attention rows without rebuilding
	// member or chat chrome.
	RefreshGuildBadges()
	// Notify surfaces an incoming message from any connected account.
	Notify(a *Account, msg store.Message)
	// ShowError surfaces an error from any account.
	ShowError(a *Account, err error)
	// Badges pushes the current per-account badge snapshot to the selector.
	Badges(badges []Badge)
}

// Badge is one row in the account selector.
type Badge struct {
	// Label is the display name (never empty; a fallback is filled in).
	Label string
	// Unread is true when the account has any unread or attention messages.
	Unread bool
	// Mentions is the account's total mention count (authoritative from the
	// backend's read state).
	Mentions int
	// Active marks the currently selected account.
	Active bool
	// Failed marks an account whose last connect attempt errored.
	Failed bool
}

// BuildFunc constructs a protocol backend for the given account keyring key,
// resolving that key's credentials and owning its store. It must not prompt
// interactively — lazy builds happen while the UI event loop owns the terminal.
// The initial active account is built before the loop starts and passed in
// prebuilt (see Seed.Backend).
type BuildFunc func(key string) (backend.Backend, error)

// Seed describes one saved account at construction time. For the launch account
// — whose interactive login, backend, store, and orchestrator are all built
// before the Manager exists (so MainView/Shell can be constructed from it) — set
// Backend and Store to that prebuilt runtime. Lazy accounts leave them nil and
// are built via BuildFunc on first activation.
type Seed struct {
	Key      string
	Label    string
	ID       uint64
	Protocol string
	Remote   string
	Backend  backend.Backend
	Store    *store.Store
}

// Account is one managed account: its saved identity plus, once built, its
// live orchestrator/store.
type Account struct {
	key      string
	label    string
	id       store.UserID
	protocol string
	remote   string

	backend backend.Backend
	store   *store.Store

	built      bool
	connecting bool
	err        error
}

// Backend returns the account's orchestrator, or nil before it is built.
func (a *Account) Backend() backend.Backend { return a.backend }

// Store returns the account's normalized store, or nil before it is built.
func (a *Account) Store() *store.Store { return a.store }

// Key returns the account's stable keyring key.
func (a *Account) Key() string { return a.key }

// ID returns the account's Discord user ID, 0 until first connect.
func (a *Account) ID() store.UserID { return a.id }

// Err returns the account's last error, if any.
func (a *Account) Err() error { return a.err }

// Label returns the account's display name, falling back to its key.
func (a *Account) Label() string {
	if a.label != "" {
		return a.label
	}
	if a.key != "" {
		return a.key
	}
	return "Account"
}

// Options configures a Manager.
type Options struct {
	UI      *tui.App
	Ctx     context.Context
	Surface Surface
	// Build constructs a protocol backend for a lazy account's key.
	Build BuildFunc
	// Persist saves the registry after a mutation (active index, discovered
	// label/id, or a new account). May be nil in tests.
	Persist func(config.Accounts)
	// EventSink, if set, receives client events (the Lua plugin system). The
	// Manager binds it to whichever account is active so plugins observe and act
	// on the active account rather than the launch account they were wired to.
	EventSink backend.EventSink
	// Seeds is the ordered set of saved accounts.
	Seeds []Seed
	// Active is the initial active index.
	Active int
	// AutoConnect enables the lazy connect goroutine; tests disable it.
	AutoConnect bool
}

// Manager owns the account set and the active selection.
type Manager struct {
	ui      *tui.App
	ctx     context.Context
	surface Surface
	build   BuildFunc
	persist func(config.Accounts)

	autoConnect bool
	accounts    []*Account
	active      int
	eventSink   backend.EventSink
}

// New builds a Manager from options. It does not connect anything; call Start.
func New(opts Options) *Manager {
	m := &Manager{
		ui:          opts.UI,
		ctx:         opts.Ctx,
		surface:     opts.Surface,
		build:       opts.Build,
		persist:     opts.Persist,
		autoConnect: opts.AutoConnect,
		active:      opts.Active,
		eventSink:   opts.EventSink,
	}
	for _, s := range opts.Seeds {
		m.accounts = append(m.accounts, &Account{
			key:      s.Key,
			label:    s.Label,
			id:       store.UserID(s.ID),
			protocol: s.Protocol,
			remote:   s.Remote,
			backend:  s.Backend,
			store:    s.Store,
		})
	}
	if m.active < 0 || m.active >= len(m.accounts) {
		m.active = 0
	}
	return m
}

// Accounts returns the managed accounts in registry order.
func (m *Manager) Accounts() []*Account { return m.accounts }

// Active returns the currently active account, or nil when there are none.
func (m *Manager) Active() *Account {
	if m.active < 0 || m.active >= len(m.accounts) {
		return nil
	}
	return m.accounts[m.active]
}

// ActiveIndex returns the active account index.
func (m *Manager) ActiveIndex() int { return m.active }

// Start builds, activates, and connects the initial active account.
func (m *Manager) Start() error {
	if len(m.accounts) == 0 {
		return ErrNoAccounts
	}
	return m.Switch(m.active)
}

// Switch activates the account at idx, building and connecting it lazily on the
// first switch. Selection (which account is visible) is client state; the
// account's own active guild/channel is restored by the UI on Activate.
func (m *Manager) Switch(idx int) error {
	if idx < 0 || idx >= len(m.accounts) {
		return fmt.Errorf("accounts: switch index %d out of range", idx)
	}
	acc := m.accounts[idx]
	if err := m.ensureBuilt(acc); err != nil {
		m.surface.ShowError(acc, err)
		m.pushBadges()
		return err
	}
	m.active = idx
	m.bindEventSink(acc)
	m.persistRegistry()
	m.surface.Activate(acc)
	m.connect(acc)
	m.pushBadges()
	return nil
}

// bindEventSink routes plugin events to the active account and detaches every
// other built account. Runs on the UI goroutine, as does App.emit, so the sink
// pointer is swapped without racing an in-flight emit.
func (m *Manager) bindEventSink(active *Account) {
	if m.eventSink == nil {
		return
	}
	for _, acc := range m.accounts {
		if acc.backend == nil {
			continue
		}
		if acc == active {
			acc.backend.SetEventSink(m.eventSink)
		} else {
			acc.backend.SetEventSink(nil)
		}
	}
}

// SwitchTo activates the given account by locating it in the registry. It lets
// callers that hold an *Account (a background-account notification) switch to it
// without tracking its index.
func (m *Manager) SwitchTo(a *Account) error {
	for i, acc := range m.accounts {
		if acc == a {
			return m.Switch(i)
		}
	}
	return fmt.Errorf("accounts: switch to unknown account")
}

// Add appends a new account to the registry (built lazily on first switch) and
// returns it. The token must already be stored under key before switching.
func (m *Manager) Add(key, label string) *Account {
	acc := &Account{key: key, label: label}
	m.accounts = append(m.accounts, acc)
	m.persistRegistry()
	m.pushBadges()
	return acc
}

// ensureBuilt constructs the account's orchestrator/store/state once. On build
// failure it records the error and leaves the account unbuilt so a later switch
// retries.
func (m *Manager) ensureBuilt(acc *Account) error {
	if acc.built {
		return nil
	}
	// Prebuilt launch account: its orchestrator/store were constructed before
	// the Manager (so MainView/Shell could bind to them). Just wire callbacks.
	if acc.backend != nil {
		if acc.store == nil {
			acc.store = acc.backend.Store()
		}
		m.wire(acc)
		acc.built = true
		return nil
	}
	if m.build == nil {
		return errors.New("accounts: no builder configured")
	}
	built, err := m.build(acc.key)
	if err != nil {
		acc.err = err
		return err
	}
	acc.backend = built
	acc.store = built.Store()
	acc.err = nil
	m.wire(acc)
	acc.built = true
	return nil
}

// wire installs the account's callbacks. Panel refreshes fire only for the
// active account; badges and notifications fire for every account.
func (m *Manager) wire(acc *Account) {
	acc.backend.OnReady(func() {
		m.hydrate(acc)
		if m.isActive(acc) {
			m.surface.Refresh()
		}
		m.pushBadges()
	})
	acc.backend.OnGuildChange(func() {
		if m.isActive(acc) {
			m.surface.Refresh()
		}
	})
	acc.backend.OnChange(func() {
		if m.isActive(acc) {
			m.surface.RefreshChannels()
		}
		m.pushBadges()
	})
	acc.backend.OnReadStateChange(func() {
		m.readStateChanged(acc)
	})
	acc.backend.OnIncomingMessage(func(msg store.Message) {
		m.surface.Notify(acc, msg)
		m.pushBadges()
	})
	acc.backend.OnError(func(err error) {
		acc.err = err
		m.surface.ShowError(acc, err)
	})
}

// connect starts the account's gateway connection once. RegisterHandlers and
// the connect goroutine must run exactly once per account, so the connecting
// flag guards re-entry from repeated switches.
func (m *Manager) connect(acc *Account) {
	if !m.autoConnect || acc.connecting || acc.backend == nil {
		return
	}
	acc.connecting = true
	acc.backend.RegisterHandlers()
	// The user-session gateway READY does not reliably deliver the guild/DM
	// directory, so this pre-connect REST pull is load-bearing (its DM
	// hydration is bounded). Lazy connect means only the accounts the user
	// actually visits pull at all, avoiding a synchronized startup burst.
	acc.backend.LoadGuilds(100)
	go func() {
		err := acc.backend.Connect(m.ctx)
		if err == nil || m.ctx.Err() != nil {
			return
		}
		m.ui.Post(func() {
			acc.err = err
			m.surface.ShowError(acc, err)
			m.pushBadges()
		})
	}()
}

// hydrate learns the account's display name and ID from the backend's self user
// once READY has populated it, persisting them for display before the next
// connect.
func (m *Manager) hydrate(acc *Account) {
	if acc.backend == nil {
		return
	}
	me, ok := acc.backend.Self()
	if !ok {
		return
	}
	changed := false
	if label := me.Name; label != "" && label != acc.label {
		acc.label = label
		changed = true
	}
	if id := me.ID; id != 0 && id != acc.id {
		acc.id = id
		changed = true
	}
	if changed {
		m.persistRegistry()
	}
}

func (m *Manager) isActive(acc *Account) bool {
	return m.Active() == acc
}

func (m *Manager) readStateChanged(acc *Account) {
	if m.isActive(acc) {
		// This path refreshes both guild and channel attention rows without
		// rebuilding members/channel chrome.
		m.surface.RefreshGuildBadges()
	}
	// Background accounts have no visible guild/channel rails, but their
	// selector badge must still follow authoritative acknowledgements/mentions.
	m.pushBadges()
}

// pushBadges rebuilds the selector badge snapshot from every account.
func (m *Manager) pushBadges() {
	badges := make([]Badge, len(m.accounts))
	for i, acc := range m.accounts {
		b := Badge{
			Label:  acc.Label(),
			Active: i == m.active,
			Failed: acc.err != nil,
		}
		if acc.backend != nil {
			b.Unread, b.Mentions = acc.backend.Unread()
		}
		badges[i] = b
	}
	m.surface.Badges(badges)
}

// persistRegistry writes the current account list and active index back to the
// saved config (tokens are never included).
func (m *Manager) persistRegistry() {
	if m.persist == nil {
		return
	}
	reg := config.Accounts{Active: m.active}
	for _, acc := range m.accounts {
		reg.List = append(reg.List, config.Account{
			Key:      acc.key,
			Label:    acc.label,
			ID:       uint64(acc.id),
			Protocol: acc.protocol,
			Remote:   acc.remote,
		})
	}
	m.persist(reg)
}
