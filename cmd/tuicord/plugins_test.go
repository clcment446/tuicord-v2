package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/discord"
	"awesomeProject/internal/plugin"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"

	"github.com/diamondburned/arikawa/v3/session"
)

// A plugin accessor must not depend on a UI Post being drained. There is no UI
// event loop in this test, matching both plugin startup and the interval after
// RunContext has returned.
func TestPluginHostAccessorsDoNotWaitForUIEventLoop(t *testing.T) {
	uiApp := tui.New()
	orch := app.New(discord.WrapSession(session.New("")), store.New(0), uiApp)
	orch.SetActive(12, 34)
	cfg := config.Default()
	cfg.ColorOverrides = &config.ColorOverrides{Rules: map[string]config.ColorRule{}}
	activeTheme, err := config.ThemeFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	host := newPluginHost(func() *app.App { return orch }, uiApp, nil, cfg.ColorOverrides, uiStyles(cfg), activeTheme)

	type ids struct {
		guild, channel, self uint64
	}
	done := make(chan ids, 1)
	go func() {
		done <- ids{
			guild:   host.ActiveGuild(),
			channel: host.ActiveChannel(),
			self:    host.SelfID(),
		}
	}()
	select {
	case got := <-done:
		if got != (ids{guild: 12, channel: 34}) {
			t.Fatalf("accessors = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("plugin accessors waited for a stopped UI event loop")
	}
}

func TestManagerCloseDoesNotWaitForStoppedUIAccessor(t *testing.T) {
	uiApp := tui.New()
	orch := app.New(discord.WrapSession(session.New("")), store.New(0), uiApp)
	cfg := config.Default()
	cfg.ColorOverrides = &config.ColorOverrides{Rules: map[string]config.ColorRule{}}
	activeTheme, err := config.ThemeFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	host := newPluginHost(func() *app.App { return orch }, uiApp, nil, cfg.ColorOverrides, uiStyles(cfg), activeTheme)
	entered := make(chan struct{})
	access := host.ActiveChannel
	host.ActiveChannel = func() uint64 {
		close(entered)
		return access()
	}

	dir := t.TempDir()
	body := `tuicord.on("read", function() tuicord.active_channel() end)`
	if err := os.WriteFile(filepath.Join(dir, "reader.lua"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr := plugin.NewManager(plugin.Options{
		Dir:             dir,
		Host:            host,
		CallbackTimeout: time.Minute,
	})
	if errs := mgr.Load(); len(errs) != 0 {
		mgr.Close()
		t.Fatalf("load errors: %v", errs)
	}
	mgr.Emit("read", nil)
	select {
	case <-entered:
	case <-time.After(time.Second):
		mgr.Close()
		t.Fatal("plugin accessor was not entered")
	}

	done := make(chan struct{})
	go func() {
		mgr.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Manager.Close deadlocked on a UI accessor after shutdown")
	}
}
