package main

import (
	"os"
	"path/filepath"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/plugin"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/ui"
)

// setupPlugins constructs the Lua plugin manager, wires its Host to the
// orchestrator/UI, loads plugins from the config directory, and registers the
// manager as the app's event sink and the shell's command/key host.
//
// It never fails startup: a missing directory or a broken plugin is reported to
// the user via a toast and logged, but the client keeps running. It returns the
// manager so the caller can Close it on shutdown, or nil when plugins are
// disabled.
func setupPlugins(orch *app.App, uiApp *tui.App, shell *ui.Shell, cfg config.Config) *plugin.Manager {
	if !cfg.PluginsEnabled() {
		return nil
	}

	pluginsDir, err := config.PluginsDir()
	if err != nil {
		shell.ShowToast("Plugins", err)
		return nil
	}
	base := filepath.Dir(pluginsDir)

	var logW *os.File
	if f, err := os.OpenFile(filepath.Join(base, "plugin.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		logW = f
	}

	host := newPluginHost(orch, uiApp, shell, cfg.ColorOverrides)

	var grants map[string][]string
	var disabled []string
	if cfg.Plugins != nil {
		grants = cfg.Plugins.Grants
		disabled = cfg.Plugins.Disabled
	}

	opts := plugin.Options{
		Dir:      pluginsDir,
		DataDir:  filepath.Join(base, "plugin-data"),
		Disabled: disabled,
		Grants:   grants,
		Host:     host,
	}
	if logW != nil {
		opts.Log = logW
	}

	mgr := plugin.NewManager(opts)
	if errs := mgr.Load(); len(errs) > 0 {
		shell.ShowToast("Plugin load", errs[0])
	}

	orch.SetEventSink(mgr)
	shell.SetPluginHost(mgr)
	return mgr
}

// newPluginHost builds the side-effecting Host the tuicord Lua API binds
// against. Every function marshals its work onto the UI goroutine via Post; the
// synchronous accessors round-trip a value back so they never read App's
// UI-goroutine-owned fields from the plugin goroutine.
func newPluginHost(orch *app.App, uiApp *tui.App, shell *ui.Shell, overrides *config.ColorOverrides) *plugin.Host {
	get := func(read func() uint64) uint64 {
		ch := make(chan uint64, 1)
		uiApp.Post(func() { ch <- read() })
		return <-ch
	}

	return &plugin.Host{
		Style: func(selector string, props map[string]string) {
			uiApp.Post(func() {
				changed := false
				for property, value := range props {
					if err := overrides.SetProperty(selector, property, value); err == nil {
						changed = true
					}
				}
				if changed {
					uiApp.Invalidate()
				}
			})
		},
		OpenOverlay: func(title string, lines []string) {
			uiApp.Post(func() {
				shell.OpenPluginOverlay(title, lines)
				uiApp.Invalidate()
			})
		},
		Send: func(content string) {
			uiApp.Post(func() { orch.Send(content) })
		},
		SendTo: func(channelID uint64, content string) {
			uiApp.Post(func() { orch.SendToChannel(store.ChannelID(channelID), content) })
		},
		Reply: func(channelID, messageID uint64, content string, mention bool) {
			uiApp.Post(func() {
				msg, ok := findMessage(orch.Store(), store.ChannelID(channelID), store.MessageID(messageID))
				if !ok {
					return
				}
				orch.Reply(content, msg, mention)
			})
		},
		React: func(channelID, messageID uint64, emoji string) {
			uiApp.Post(func() {
				orch.AddReaction(store.ChannelID(channelID), store.MessageID(messageID), emoji)
			})
		},
		Notify: func(title, body string) {
			uiApp.Post(func() { shell.ShowNotice(title, body) })
		},
		ActiveChannel: func() uint64 { return get(func() uint64 { return uint64(orch.ActiveChannel()) }) },
		ActiveGuild:   func() uint64 { return get(func() uint64 { return uint64(orch.ActiveGuild()) }) },
		SelfID:        func() uint64 { return get(func() uint64 { return uint64(orch.SelfID()) }) },
	}
}

// findMessage locates a message by ID within a channel's loaded history.
func findMessage(st *store.Store, channel store.ChannelID, id store.MessageID) (store.Message, bool) {
	if st == nil {
		return store.Message{}, false
	}
	for _, m := range st.Messages(channel) {
		if m.ID == id {
			return m, true
		}
	}
	return store.Message{}, false
}
