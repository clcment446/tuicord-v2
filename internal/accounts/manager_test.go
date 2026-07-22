package accounts

import (
	"context"
	"errors"
	"testing"

	"awesomeProject/internal/config"
	"awesomeProject/internal/discord"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"

	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/ningen/v3"
)

// fakeSurface records Manager-driven UI effects for assertions.
type fakeSurface struct {
	activated    []*Account
	refresh      int
	refreshChan  int
	refreshGuild int
	notifies     []*Account
	errors       []error
	lastBadges   []Badge
}

func (f *fakeSurface) Activate(a *Account)                { f.activated = append(f.activated, a) }
func (f *fakeSurface) Refresh()                           { f.refresh++ }
func (f *fakeSurface) RefreshChannels()                   { f.refreshChan++ }
func (f *fakeSurface) RefreshGuildBadges()                { f.refreshGuild++ }
func (f *fakeSurface) Notify(a *Account, _ store.Message) { f.notifies = append(f.notifies, a) }
func (f *fakeSurface) ShowError(_ *Account, err error)    { f.errors = append(f.errors, err) }
func (f *fakeSurface) Badges(b []Badge)                   { f.lastBadges = b }

// recordingSink records the account the currently-bound plugin events come from.
type recordingSink struct{ events int }

func (r *recordingSink) Emit(string, map[string]any) { r.events++ }

// TestEventSinkFollowsActiveAccount proves plugin events are delivered only from
// the active account, so a switch moves plugin observation to the new account.
func TestEventSinkFollowsActiveAccount(t *testing.T) {
	surf := &fakeSurface{}
	sink := &recordingSink{}
	seeds := []Seed{
		{Key: "token", Label: "Alice", Ning: wrapState(t)},
		{Key: "acct-2", Label: "Bob", Ning: wrapState(t)},
	}
	m := New(Options{
		UI: tui.New(), Ctx: context.Background(), Surface: surf,
		Persist: func(config.Accounts) {}, Seeds: seeds, Active: 0,
		AutoConnect: false, EventSink: sink,
	})
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Account 0 is active: SetActive emits "channel.switch", which reaches the sink.
	a0 := m.Accounts()[0].App()
	a0.SetActive(1, 1)
	if sink.events == 0 {
		t.Fatal("active account events did not reach the sink")
	}

	if err := m.Switch(1); err != nil {
		t.Fatalf("Switch(1): %v", err)
	}
	before := sink.events
	a0.SetActive(2, 2) // account 0 is now a background account: detached
	if sink.events != before {
		t.Fatal("background account events still reached the sink")
	}
	m.Accounts()[1].App().SetActive(3, 3) // newly active account: attached
	if sink.events == before {
		t.Fatal("newly active account events did not reach the sink")
	}
}

// wrapState builds a ningen state around an offline fake session so app.New
// works without any network. The connection is never opened in these tests.
func wrapState(t *testing.T) *ningen.State {
	t.Helper()
	return discord.WrapSession(session.New(""))
}

func newManager(t *testing.T, surf Surface, seeds []Seed, build BuildFunc) (*Manager, *[]config.Accounts) {
	t.Helper()
	var saved []config.Accounts
	m := New(Options{
		UI:          tui.New(),
		Ctx:         context.Background(),
		Surface:     surf,
		Build:       build,
		Persist:     func(a config.Accounts) { saved = append(saved, a) },
		Seeds:       seeds,
		Active:      0,
		AutoConnect: false, // never open a real gateway in unit tests
	})
	return m, &saved
}

func TestStartActivatesAndPersistsActiveAccount(t *testing.T) {
	surf := &fakeSurface{}
	seeds := []Seed{
		{Key: "token", Label: "Alice", Ning: wrapState(t)},
		{Key: "acct-2", Label: "Bob", Ning: wrapState(t)},
	}
	m, saved := newManager(t, surf, seeds, nil)

	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(surf.activated) != 1 || surf.activated[0] != m.Accounts()[0] {
		t.Fatalf("expected account 0 activated, got %v", surf.activated)
	}
	if m.Active() != m.Accounts()[0] {
		t.Fatalf("active account = %v, want index 0", m.Active())
	}
	if len(surf.lastBadges) != 2 || !surf.lastBadges[0].Active || surf.lastBadges[1].Active {
		t.Fatalf("badges active flags wrong: %+v", surf.lastBadges)
	}
	if len(*saved) == 0 || (*saved)[len(*saved)-1].Active != 0 {
		t.Fatalf("expected persisted active 0, got %+v", *saved)
	}
}

func TestSwitchChangesActiveAndRebinds(t *testing.T) {
	surf := &fakeSurface{}
	seeds := []Seed{
		{Key: "token", Label: "Alice", Ning: wrapState(t)},
		{Key: "acct-2", Label: "Bob", Ning: wrapState(t)},
	}
	m, saved := newManager(t, surf, seeds, nil)
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := m.Switch(1); err != nil {
		t.Fatalf("Switch(1): %v", err)
	}
	if m.ActiveIndex() != 1 || m.Active() != m.Accounts()[1] {
		t.Fatalf("active index = %d, want 1", m.ActiveIndex())
	}
	if surf.activated[len(surf.activated)-1] != m.Accounts()[1] {
		t.Fatalf("last activated account is not account 1")
	}
	if got := (*saved)[len(*saved)-1].Active; got != 1 {
		t.Fatalf("persisted active = %d, want 1", got)
	}
	if !surf.lastBadges[1].Active || surf.lastBadges[0].Active {
		t.Fatalf("badge active flag did not follow switch: %+v", surf.lastBadges)
	}
}

func TestLazyBuildHappensOnceAcrossSwitches(t *testing.T) {
	surf := &fakeSurface{}
	// Account 1 has no prebuilt state, so it must be built via BuildFunc.
	seeds := []Seed{
		{Key: "token", Label: "Alice", Ning: wrapState(t)},
		{Key: "acct-2", Label: "Bob"},
	}
	builds := map[string]int{}
	build := func(key string) (*ningen.State, error) {
		builds[key]++
		return wrapState(t), nil
	}
	m, _ := newManager(t, surf, seeds, build)
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if builds["token"] != 0 {
		t.Fatalf("prebuilt account should not be built, got %d", builds["token"])
	}

	for i := 0; i < 3; i++ {
		if err := m.Switch(1); err != nil {
			t.Fatalf("Switch(1) #%d: %v", i, err)
		}
		if err := m.Switch(0); err != nil {
			t.Fatalf("Switch(0) #%d: %v", i, err)
		}
	}
	if builds["acct-2"] != 1 {
		t.Fatalf("lazy account built %d times, want exactly 1", builds["acct-2"])
	}
	if m.Accounts()[1].App() == nil {
		t.Fatalf("lazy account was not built")
	}
}

func TestSwitchBuildFailureSurfacesErrorAndRetries(t *testing.T) {
	surf := &fakeSurface{}
	seeds := []Seed{
		{Key: "token", Label: "Alice", Ning: wrapState(t)},
		{Key: "acct-2", Label: "Bob"},
	}
	attempts := 0
	build := func(key string) (*ningen.State, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("keyring locked")
		}
		return wrapState(t), nil
	}
	m, _ := newManager(t, surf, seeds, build)
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := m.Switch(1); err == nil {
		t.Fatalf("expected first Switch(1) to fail")
	}
	if len(surf.errors) == 0 {
		t.Fatalf("expected error surfaced to UI")
	}
	if m.ActiveIndex() != 0 {
		t.Fatalf("active should stay 0 after failed switch, got %d", m.ActiveIndex())
	}
	// A retry rebuilds (the account was not marked built on failure).
	if err := m.Switch(1); err != nil {
		t.Fatalf("retry Switch(1): %v", err)
	}
	if m.ActiveIndex() != 1 {
		t.Fatalf("retry did not activate account 1")
	}
}

func TestBadgeReflectsUnreadFromStore(t *testing.T) {
	surf := &fakeSurface{}
	seeds := []Seed{{Key: "token", Label: "Alice", Ning: wrapState(t)}}
	m, _ := newManager(t, surf, seeds, nil)
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if surf.lastBadges[0].Unread {
		t.Fatalf("account should start with no unread")
	}
	// Simulate an unread attention message in the account's store, then force a
	// badge refresh via a second switch.
	m.Accounts()[0].Store().IncrementPing(store.ChannelID(5))
	if err := m.Switch(0); err != nil {
		t.Fatalf("Switch(0): %v", err)
	}
	if !surf.lastBadges[0].Unread {
		t.Fatalf("badge did not reflect store attention message: %+v", surf.lastBadges[0])
	}
}

func TestBackgroundReadStateChangePushesAccountBadges(t *testing.T) {
	surf := &fakeSurface{}
	seeds := []Seed{
		{Key: "token", Label: "Alice", Ning: wrapState(t)},
		{Key: "acct-2", Label: "Bob", Ning: wrapState(t)},
	}
	m, _ := newManager(t, surf, seeds, nil)
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Switch(1); err != nil {
		t.Fatalf("Switch(1): %v", err)
	}
	if err := m.Switch(0); err != nil {
		t.Fatalf("Switch(0): %v", err)
	}
	background := m.Accounts()[1]
	background.Store().IncrementPing(5)
	guildRefreshes := surf.refreshGuild

	m.readStateChanged(background)

	if len(surf.lastBadges) != 2 || !surf.lastBadges[1].Unread {
		t.Fatalf("background badge was not refreshed: %+v", surf.lastBadges)
	}
	if surf.refreshGuild != guildRefreshes {
		t.Fatalf("background read state refreshed active guild rail: got %d, want %d", surf.refreshGuild, guildRefreshes)
	}
}

func TestActiveReadStateChangeUsesAttentionOnlyRefresh(t *testing.T) {
	surf := &fakeSurface{}
	m, _ := newManager(t, surf, []Seed{{Key: "token", Ning: wrapState(t)}}, nil)
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	channels, guilds := surf.refreshChan, surf.refreshGuild

	m.readStateChanged(m.Active())

	if surf.refreshChan != channels || surf.refreshGuild != guilds+1 {
		t.Fatalf("full-channel/guild refreshes = %d/%d, want %d/%d", surf.refreshChan, surf.refreshGuild, channels, guilds+1)
	}
}

func TestBadgeLabelFallsBackToKey(t *testing.T) {
	surf := &fakeSurface{}
	seeds := []Seed{{Key: "acct-xyz", Ning: wrapState(t)}}
	m, _ := newManager(t, surf, seeds, nil)
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := surf.lastBadges[0].Label; got != "acct-xyz" {
		t.Fatalf("badge label = %q, want key fallback", got)
	}
}

func TestStartWithNoAccountsErrors(t *testing.T) {
	surf := &fakeSurface{}
	m, _ := newManager(t, surf, nil, nil)
	if err := m.Start(); !errors.Is(err, ErrNoAccounts) {
		t.Fatalf("Start() error = %v, want ErrNoAccounts", err)
	}
}
