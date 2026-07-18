package plugin

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// Options configures a Manager. It is deliberately built from plain values so
// this package needs no dependency on internal/config; the wiring layer
// translates the user's config into these fields.
type Options struct {
	// Dir is the directory scanned for plugins (*.lua files or <name>/init.lua).
	Dir string
	// DataDir is the base directory under which each plugin gets its own
	// data folder for the "fs" grant. Empty disables fs even when granted.
	DataDir string
	// Disabled lists plugin names to skip.
	Disabled []string
	// Grants maps a plugin name to its granted capabilities ("fs", "net").
	Grants map[string][]string
	// Host supplies the side-effecting operations bindings call.
	Host *Host
	// Log receives plugin log/print output and load diagnostics. Nil discards.
	Log io.Writer
	// QueueSize bounds the plugin job queue; <=0 uses a sane default.
	QueueSize int
}

// Manager owns the Lua runtime and the plugin registries. It is safe to call
// Emit, RunCommand and RunKey from any goroutine; loading and Close should be
// called from a single owner goroutine (typically the wiring layer).
type Manager struct {
	opts     Options
	rt       *runtime
	events   *eventBus
	commands *commandRegistry
	keys     *keyRegistry
	themes   *themeRegistry
	host     *Host

	mu     sync.Mutex
	states []*lua.LState
	loaded []string
}

// NewManager creates a started Manager. Call Load to discover and run plugins.
func NewManager(opts Options) *Manager {
	if opts.Host == nil {
		opts.Host = &Host{}
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = 256
	}
	m := &Manager{
		opts:     opts,
		rt:       newRuntime(opts.QueueSize),
		events:   newEventBus(),
		commands: newCommandRegistry(),
		keys:     newKeyRegistry(),
		themes:   newThemeRegistry(),
		host:     opts.Host,
	}
	m.rt.start()
	return m
}

// Loaded returns the names of successfully loaded plugins, sorted.
func (m *Manager) Loaded() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]string(nil), m.loaded...)
	sort.Strings(out)
	return out
}

// Load discovers plugin files under opts.Dir and runs each in its own
// sandboxed state. It returns one error per plugin that failed to load; a
// failure isolates that plugin without affecting the others. A missing plugins
// directory is not an error (returns nil).
func (m *Manager) Load() []error {
	files, err := discover(m.opts.Dir)
	if err != nil {
		return []error{err}
	}
	var errs []error
	for _, f := range files {
		if m.isDisabled(f.name) {
			m.logf("plugin %q disabled by config", f.name)
			continue
		}
		if err := m.loadOne(f); err != nil {
			m.logf("plugin %q failed: %v", f.name, err)
			errs = append(errs, fmt.Errorf("%s: %w", f.name, err))
			continue
		}
		m.mu.Lock()
		m.loaded = append(m.loaded, f.name)
		m.mu.Unlock()
		m.logf("plugin %q loaded", f.name)
	}
	return errs
}

// LoadConfig runs a single Lua config file (e.g. config.lua) as a context named
// "config". Unlike Load it is not gated on the plugins toggle: it is the seam
// for user settings and keybindings expressed in Lua rather than as a plugin. A
// missing file is not an error.
func (m *Manager) LoadConfig(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := m.loadOne(pluginFile{name: "config", path: path}); err != nil {
		m.logf("config.lua failed: %v", err)
		return err
	}
	m.mu.Lock()
	m.loaded = append(m.loaded, "config")
	m.mu.Unlock()
	m.logf("config.lua loaded")
	return nil
}

// loadOne creates a state, installs the API, and runs the plugin file. All Lua
// work happens on the plugin goroutine.
func (m *Manager) loadOne(f pluginFile) error {
	var loadErr error
	ok := m.rt.do(func() {
		L, err := newSandboxedState()
		if err != nil {
			loadErr = err
			return
		}
		pctx := &pluginContext{
			name:     f.name,
			host:     m.host,
			events:   m.events,
			commands: m.commands,
			keys:     m.keys,
			themes:   m.themes,
			log:      func(msg string) { m.logf("[%s] %s", f.name, msg) },
			grants:   grantSet(m.opts.Grants[f.name]),
			dataDir:  m.pluginDataDir(f.name),
		}
		installAPI(L, pctx)
		if err := L.DoFile(f.path); err != nil {
			L.Close()
			loadErr = err
			return
		}
		m.mu.Lock()
		m.states = append(m.states, L)
		m.mu.Unlock()
	})
	if !ok {
		return fmt.Errorf("runtime stopped")
	}
	return loadErr
}

// Emit dispatches an event to subscribed plugins. It is non-blocking and safe
// to call from gateway/UI goroutines; if the queue is full the event is dropped
// (and logged) so a stuck plugin cannot back-pressure the caller.
func (m *Manager) Emit(name string, payload map[string]any) {
	if m == nil {
		return
	}
	if !m.rt.submit(func() {
		m.events.dispatch(name, payload, m.onCallbackError)
	}) {
		m.logf("dropped event %q (plugin queue full)", name)
	}
}

// RunCommand runs a plugin-registered ;-command by name. It reports whether a
// command with that name exists (so the caller can fall through to "unknown"),
// and dispatches the handler asynchronously on the plugin goroutine.
func (m *Manager) RunCommand(name string, args []string) bool {
	if m == nil {
		return false
	}
	h, ok := m.commands.lookup(strings.ToLower(name))
	if !ok {
		return false
	}
	m.rt.submit(func() {
		argsTbl := h.L.NewTable()
		for _, a := range args {
			argsTbl.Append(lua.LString(a))
		}
		if err := safeCall(h.L, h.fn, argsTbl); err != nil {
			m.onCallbackError(h.plugin, err)
		}
	})
	return true
}

// ThemeNames returns the registered theme names, sorted, for a ;theme listing.
func (m *Manager) ThemeNames() []string {
	if m == nil {
		return nil
	}
	return m.themes.names()
}

// ApplyTheme applies a registered theme by name via the Host, reporting whether
// a theme with that name exists.
func (m *Manager) ApplyTheme(name string) bool {
	if m == nil {
		return false
	}
	palette, ok := m.themes.lookup(name)
	if !ok {
		return false
	}
	if m.host != nil && m.host.ApplyTheme != nil {
		m.host.ApplyTheme(palette)
	}
	return true
}

// KeySpecs returns the key specs plugins have bound, so the UI can match an
// incoming key against them.
func (m *Manager) KeySpecs() []string {
	if m == nil {
		return nil
	}
	return m.keys.specs()
}

// RunKey runs a plugin-bound key handler. It reports whether the spec is bound.
func (m *Manager) RunKey(spec string) bool {
	if m == nil {
		return false
	}
	h, ok := m.keys.lookup(spec)
	if !ok {
		return false
	}
	m.rt.submit(func() {
		if err := safeCall(h.L, h.fn); err != nil {
			m.onCallbackError(h.plugin, err)
		}
	})
	return true
}

// Commands returns the registered plugin commands for a help listing.
func (m *Manager) Commands() []entry {
	if m == nil {
		return nil
	}
	return m.commands.list()
}

// CommandNames returns the registered plugin command names, sorted. It exists
// so consumers can list commands without referencing the unexported entry type.
func (m *Manager) CommandNames() []string {
	if m == nil {
		return nil
	}
	entries := m.commands.list()
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}

// Close stops the runtime and releases every plugin state. After Close the
// Manager must not be reused.
func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.rt.stop()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, L := range m.states {
		L.Close()
	}
	m.states = nil
}

func (m *Manager) onCallbackError(plugin string, err error) {
	m.logf("[%s] error: %v", plugin, err)
	if m.host != nil && m.host.Notify != nil {
		m.host.Notify("Plugin error: "+plugin, err.Error())
	}
}

func (m *Manager) isDisabled(name string) bool {
	for _, d := range m.opts.Disabled {
		if d == name {
			return true
		}
	}
	return false
}

func (m *Manager) pluginDataDir(name string) string {
	if m.opts.DataDir == "" {
		return ""
	}
	return filepath.Join(m.opts.DataDir, name)
}

func (m *Manager) logf(format string, args ...any) {
	if m.opts.Log == nil {
		return
	}
	fmt.Fprintf(m.opts.Log, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

// grantSet turns a capability list into a lookup set.
func grantSet(caps []string) map[string]bool {
	set := make(map[string]bool, len(caps))
	for _, c := range caps {
		set[c] = true
	}
	return set
}

// pluginFile is a discovered plugin: its display name and entry file path.
type pluginFile struct {
	name string
	path string
}

// discover lists plugins in dir. A top-level "foo.lua" becomes plugin "foo"; a
// subdirectory "bar/" containing init.lua becomes plugin "bar". A missing dir
// yields no plugins and no error.
func discover(dir string) ([]pluginFile, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []pluginFile
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			init := filepath.Join(dir, name, "init.lua")
			if info, err := os.Stat(init); err == nil && !info.IsDir() {
				files = append(files, pluginFile{name: name, path: init})
			}
			continue
		}
		if strings.HasSuffix(name, ".lua") {
			base := strings.TrimSuffix(name, ".lua")
			files = append(files, pluginFile{name: base, path: filepath.Join(dir, name)})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return files, nil
}
