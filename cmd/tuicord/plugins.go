package main

import (
	"os"
	"path/filepath"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/plugin"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/ui"
)

// setupPlugins constructs the Lua plugin manager, wires its Host to the
// orchestrator/UI, loads the optional config.lua and the plugins directory, and
// registers the manager as the app's event sink and the shell's command/key
// host.
//
// config.lua is loaded whenever it exists, independent of the plugins toggle,
// so users can express settings and keybindings in Lua without writing a
// plugin. The plugins directory is loaded only when [plugins].enabled.
//
// It never fails startup: a missing file or a broken plugin is reported to the
// user via a toast and logged, but the client keeps running. It returns the
// manager so the caller can Close it on shutdown, or nil when there is nothing
// to load.
func setupPlugins(orch *app.App, uiApp *tui.App, shell *ui.Shell, cfg config.Config, styles ui.Styles) *plugin.Manager {
	pluginsDir, err := config.PluginsDir()
	if err != nil {
		shell.ShowToast("Plugins", err)
		return nil
	}
	base := filepath.Dir(pluginsDir)
	configLua := filepath.Join(base, "config.lua")

	_, statErr := os.Stat(configLua)
	hasConfigLua := statErr == nil
	if !cfg.PluginsEnabled() && !hasConfigLua {
		return nil
	}

	var logW *os.File
	if f, err := os.OpenFile(filepath.Join(base, "plugin.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		logW = f
	}

	host := newPluginHost(orch, uiApp, shell, cfg.ColorOverrides, styles.Cells, cfg.Colors)

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
	// config.lua first: its settings/keybinds apply even when plugins are off.
	if err := mgr.LoadConfig(configLua); err != nil {
		shell.ShowToast("config.lua", err)
	}
	if cfg.PluginsEnabled() {
		if errs := mgr.Load(); len(errs) > 0 {
			shell.ShowToast("Plugin load", errs[0])
		}
	}

	orch.SetEventSink(mgr)
	shell.SetPluginHost(mgr)
	return mgr
}

// newPluginHost builds the side-effecting Host the tuicord Lua API binds
// against. Every function marshals its work onto the UI goroutine via Post; the
// synchronous accessors round-trip a value back so they never read App's
// UI-goroutine-owned fields from the plugin goroutine.
func newPluginHost(orch *app.App, uiApp *tui.App, shell *ui.Shell, overrides *config.ColorOverrides, cells map[string]screen.Style, palette config.Colors) *plugin.Host {
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
		ApplyTheme: func(p map[string]string) {
			uiApp.Post(func() {
				palette = mergePalette(palette, p)
				fresh := config.CellStyles(palette.Styles(), overrides)
				// Repopulate the shared cells map in place so every widget that
				// resolves via Styles.Cell picks up the new palette on the next
				// render, without rebuilding the widget tree.
				for k := range cells {
					delete(cells, k)
				}
				for k, v := range fresh {
					cells[k] = v
				}
				uiApp.Invalidate()
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

// mergePalette overlays hex values from a plugin theme onto the current palette.
// Keys mirror config.Colors fields; unknown or empty values are ignored.
func mergePalette(base config.Colors, p map[string]string) config.Colors {
	set := func(dst *string, key string) {
		if v, ok := p[key]; ok && v != "" {
			*dst = v
		}
	}
	set(&base.Background, "background")
	set(&base.Text, "text")
	set(&base.Muted, "muted")
	set(&base.Accent, "accent")
	set(&base.Selection, "selection")
	set(&base.Border, "border")
	set(&base.Error, "error")
	return base
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
