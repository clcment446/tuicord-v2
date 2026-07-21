package main

import (
	"fmt"
	"os"
	"path/filepath"

	"awesomeProject/internal/app"
	"awesomeProject/internal/config"
	"awesomeProject/internal/plugin"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/ui"
)

// newBootstrapPluginManager creates the single process-wide Lua manager before
// login or any UI object. Its Host is intentionally inert; config mutation and
// startup theme selection are synchronous typed operations owned by Manager.
func newBootstrapPluginManager(cfg *config.Config) (*plugin.Manager, *os.File, error) {
	pluginsDir, err := config.PluginsDir()
	if err != nil {
		return nil, nil, err
	}
	base := filepath.Dir(pluginsDir)
	var logW *os.File
	if f, err := os.OpenFile(filepath.Join(base, "plugin.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		logW = f
	}
	opts := plugin.Options{
		Dir:          pluginsDir,
		DataDir:      filepath.Join(base, "plugin-data"),
		Host:         &plugin.Host{},
		ConfigTarget: cfg,
	}
	if logW != nil {
		opts.Log = logW
	}
	return plugin.NewManager(opts), logW, nil
}

// attachAndLoadPlugins preserves config.lua registrations by populating the
// manager's bootstrap Host in place, then loads ordinary plugins under the
// Lua-derived policy. Plugin startup side effects Post to the now-live UI Host.
func attachAndLoadPlugins(mgr *plugin.Manager, orch *app.App, uiApp *tui.App, shell *ui.Shell, cfg config.Config, styles ui.Styles, activeTheme config.Theme) []error {
	if mgr == nil {
		return nil
	}
	mgr.AttachHost(newPluginHost(orch, uiApp, shell, cfg.ColorOverrides, styles, activeTheme))
	orch.SetEventSink(mgr)
	shell.SetPluginHost(mgr)

	var disabled []string
	var grants map[string][]string
	if cfg.Plugins != nil {
		disabled = cfg.Plugins.Disabled
		grants = cfg.Plugins.Grants
	}
	mgr.SetPluginConfig(disabled, grants)
	if !cfg.PluginsEnabled() {
		return nil
	}
	return mgr.Load()
}

// newPluginHost builds the live side-effecting Host. Every UI/store mutation is
// posted; synchronous accessors use app's immutable atomic snapshot and never
// wait for an event loop that may not be running.
func newPluginHost(orch *app.App, uiApp *tui.App, shell *ui.Shell, overrides *config.ColorOverrides, styles ui.Styles, activeTheme config.Theme) *plugin.Host {
	install := func(theme config.Theme) {
		activeTheme = theme
		overrides.Replace(theme.Styles)
		fresh := config.CellStyles(theme.Palette.Styles(), overrides)
		styles.Install(fresh, config.CustomCellKeys(fresh, overrides))
		if shell != nil {
			shell.SetStyles(styles)
		}
		uiApp.SetTheme(tuiTheme(theme.Palette, overrides))
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
					fresh := config.CellStyles(activeTheme.Palette.Styles(), overrides)
					styles.Install(fresh, config.CustomCellKeys(fresh, overrides))
					if shell != nil {
						shell.SetStyles(styles)
					}
					uiApp.Invalidate()
				}
			})
		},
		ApplyTheme: func(theme config.Theme) {
			uiApp.Post(func() { install(theme) })
		},
		OpenOverlay: func(title string, lines []string) {
			uiApp.Post(func() {
				if shell != nil {
					shell.OpenPluginOverlay(title, lines)
				}
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
				if ok {
					orch.Reply(content, msg, mention)
				}
			})
		},
		React: func(channelID, messageID uint64, emoji string) {
			uiApp.Post(func() { orch.AddReaction(store.ChannelID(channelID), store.MessageID(messageID), emoji) })
		},
		Notify: func(title, body string) {
			uiApp.Post(func() {
				if shell != nil {
					shell.ShowNotice(title, body)
				}
			})
		},
		ActiveChannel: func() uint64 { return uint64(orch.Snapshot().ActiveChannel) },
		ActiveGuild:   func() uint64 { return uint64(orch.Snapshot().ActiveGuild) },
		SelfID:        func() uint64 { return uint64(orch.Snapshot().SelfID) },
	}
}

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

func pluginLoadError(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%d plugin(s) failed; first: %w", len(errs), errs[0])
}
