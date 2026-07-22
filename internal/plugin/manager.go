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

	"awesomeProject/internal/config"
	lua "github.com/yuin/gopher-lua"
)

// Options configures a Manager. ConfigTarget is the one typed integration
// point for declarative config.lua; UI and Discord operations remain decoupled
// behind Host functions.
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
	// Host supplies the side-effecting operations bindings call. It may be a
	// bootstrap-safe empty Host and populated later with AttachHost.
	Host *Host
	// ConfigTarget is the typed runtime configuration configured by config.lua.
	// It is ignored for ordinary plugin files.
	ConfigTarget *config.Config
	// Log receives plugin log/print output and load diagnostics. Nil discards.
	Log io.Writer
	// QueueSize bounds the plugin job queue; <=0 uses a sane default.
	QueueSize int
	// StartupTimeout bounds execution of each plugin/config file. <=0 uses 5s.
	StartupTimeout time.Duration
	// CallbackTimeout bounds each event, command, or key callback. <=0 uses 5s.
	CallbackTimeout time.Duration
}

// Manager owns the Lua runtime and the plugin registries. Emit, RunCommand,
// RunKey, and Close are safe from any goroutine (including concurrent Close
// calls). Loading must finish before normal dispatch or shutdown begins.
type Manager struct {
	opts     Options
	rt       *runtime
	events   *eventBus
	commands *commandRegistry
	keys     *keyRegistry
	themes   *themeRegistry
	timers   *timerRegistry
	host     *Host

	mu                   sync.Mutex
	states               []managedState
	loaded               []string
	startupTheme         string
	startupThemeConsumed bool

	logMu sync.Mutex
}

type managedState struct {
	L      *lua.LState
	fsRoot *os.Root
}

// NewManager creates a started Manager. Call Load to discover and run plugins.
func NewManager(opts Options) *Manager {
	if opts.Host == nil {
		opts.Host = &Host{}
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = 256
	}
	if opts.StartupTimeout <= 0 {
		opts.StartupTimeout = defaultStartupTimeout
	}
	if opts.CallbackTimeout <= 0 {
		opts.CallbackTimeout = defaultCallbackTimeout
	}
	m := &Manager{
		opts:     opts,
		rt:       newRuntime(opts.QueueSize),
		events:   newEventBus(),
		commands: newCommandRegistry(),
		keys:     newKeyRegistry(),
		themes:   newThemeRegistry(),
		timers:   &timerRegistry{},
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
	if err := m.loadOne(pluginFile{name: "config", path: path, configContext: true}); err != nil {
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
		grants := grantSet(m.opts.Grants[f.name])
		var fsRoot *os.Root
		if grants[CapFS] && m.opts.DataDir != "" {
			fsRoot, err = m.openPluginFSRoot(f.name)
			if err != nil {
				L.Close()
				loadErr = fmt.Errorf("open filesystem grant: %w", err)
				return
			}
		}
		pctx := &pluginContext{
			name:          f.name,
			host:          m.host,
			events:        m.events,
			commands:      m.commands,
			keys:          m.keys,
			themes:        m.themes,
			log:           func(msg string) { m.logf("[%s] %s", f.name, msg) },
			grants:        grants,
			fsRoot:        fsRoot,
			configContext: f.configContext,
			starting:      true,
			applyTheme:    m.applyTheme,
			every: func(interval time.Duration, fn *lua.LFunction, L *lua.LState) {
				m.timers.every(interval, L, func() {
					if !m.rt.submit(func() {
						// A tick that fired during startup can be queued before the
						// state is rolled back and closed; never call into a state
						// that was not committed.
						if !m.isLive(L) {
							return
						}
						if err := safeCall(m.rt.context(), L, fn, m.opts.CallbackTimeout); err != nil {
							m.onCallbackError(f.name, err)
						}
					}) {
						m.logf("dropped timer for plugin %q", f.name)
					}
				})
			},
			submit:          m.rt.submit,
			context:         m.rt.context,
			callbackTimeout: m.opts.CallbackTimeout,
			onCallbackError: func(err error) { m.onCallbackError(f.name, err) },
			isLive:          m.isLive,
		}
		var workingConfig config.Config
		if f.configContext && m.opts.ConfigTarget != nil {
			workingConfig = *m.opts.ConfigTarget
			pctx.configTarget = &workingConfig
		}
		installAPI(L, pctx)
		if err := safeDoFile(m.rt.context(), L, f.path, m.opts.StartupTimeout); err != nil {
			// Startup registration is transactional. Roll back everything owned by
			// this state (revealing any prior owners it shadowed) before closing it.
			m.rollbackRegistrations(L)
			if fsRoot != nil {
				_ = fsRoot.Close()
			}
			L.Close()
			loadErr = err
			return
		}
		pctx.starting = false
		if pctx.configTarget != nil {
			*m.opts.ConfigTarget = workingConfig
		}
		m.mu.Lock()
		m.states = append(m.states, managedState{L: L, fsRoot: fsRoot})
		if pctx.startupTheme != "" {
			m.startupTheme = pctx.startupTheme
			m.startupThemeConsumed = false
		}
		m.mu.Unlock()
		if pctx.runtimeTheme != "" {
			_ = m.applyTheme(pctx.runtimeTheme)
		}
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
		m.events.dispatch(m.rt.context(), m.opts.CallbackTimeout, name, payload, m.onCallbackError)
	}) {
		m.logf("dropped event %q (plugin queue full or runtime stopping)", name)
	}
}

// RunCommand runs a plugin-registered ;-command by name. It reports whether
// the callback was accepted (so the caller can fall through when it was not)
// and dispatches the handler asynchronously on the plugin goroutine.
func (m *Manager) RunCommand(name string, args []string) bool {
	if m == nil {
		return false
	}
	h, ok := m.commands.lookup(strings.ToLower(name))
	if !ok {
		return false
	}
	if !m.rt.submit(func() {
		argsTbl := h.L.NewTable()
		for _, a := range args {
			argsTbl.Append(lua.LString(a))
		}
		if err := safeCall(m.rt.context(), h.L, h.fn, m.opts.CallbackTimeout, argsTbl); err != nil {
			m.onCallbackError(h.plugin, err)
		}
	}) {
		m.logf("dropped command %q (plugin queue full or runtime stopping)", name)
		return false
	}
	return true
}

// ThemeNames returns the registered theme names, sorted, for a ;theme listing.
func (m *Manager) ThemeNames() []string {
	if m == nil {
		return nil
	}
	return m.themes.names()
}

// ApplyTheme applies a registered theme by name via the live Host, reporting
// whether a theme with that name exists.
func (m *Manager) ApplyTheme(name string) bool {
	return m != nil && m.applyTheme(name) == nil
}

func (m *Manager) applyTheme(name string) error {
	theme, ok := m.themes.lookup(name)
	if !ok {
		return fmt.Errorf("unknown theme %q", name)
	}
	if m.host != nil && m.host.ApplyTheme != nil {
		m.host.ApplyTheme(theme)
	}
	return nil
}

// ConsumeStartupTheme resolves the config.lua startup selection and marks it as
// already projected into Config. Main calls this before constructing login or
// any widget, preventing a redundant bootstrap Post when the live Host attaches.
func (m *Manager) ConsumeStartupTheme() (string, config.Theme, bool) {
	if m == nil {
		return "", config.Theme{}, false
	}
	m.mu.Lock()
	name := m.startupTheme
	if name != "" {
		m.startupThemeConsumed = true
	}
	m.mu.Unlock()
	if name == "" {
		return "", config.Theme{}, false
	}
	theme, ok := m.themes.lookup(name)
	return name, theme, ok
}

// AttachHost populates the bootstrap Host in place so commands and keymaps
// registered by config.lua keep working. If the caller did not consume the
// startup theme into Config, attachment applies it through the live Host.
func (m *Manager) AttachHost(host *Host) {
	if m == nil || host == nil {
		return
	}
	// Copy the host on the runtime goroutine so the field assignment cannot race
	// a Lua callback that is dereferencing m.host (every callback runs there). If
	// the runtime is already stopping, no callback can run, so assign directly.
	hostCopy := *host
	if !m.rt.do(func() { *m.host = hostCopy }) {
		*m.host = hostCopy
	}
	m.mu.Lock()
	name := m.startupTheme
	apply := name != "" && !m.startupThemeConsumed
	m.mu.Unlock()
	if apply {
		_ = m.applyTheme(name)
	}
}

// SetPluginConfig updates discovery policy after config.lua has populated the
// typed Config. Call it before Load.
func (m *Manager) SetPluginConfig(disabled []string, grants map[string][]string) {
	if m == nil {
		return
	}
	m.opts.Disabled = append([]string(nil), disabled...)
	m.opts.Grants = make(map[string][]string, len(grants))
	for name, values := range grants {
		m.opts.Grants[name] = append([]string(nil), values...)
	}
}

// KeySpecs returns the key specs plugins have bound, so the UI can match an
// incoming key against them.
func (m *Manager) KeySpecs() []string {
	if m == nil {
		return nil
	}
	return m.keys.specs()
}

// RunKey runs a plugin-bound key handler. It reports whether the callback was
// accepted for asynchronous execution.
func (m *Manager) RunKey(spec string) bool {
	if m == nil {
		return false
	}
	h, ok := m.keys.lookup(spec)
	if !ok {
		return false
	}
	if !m.rt.submit(func() {
		if err := safeCall(m.rt.context(), h.L, h.fn, m.opts.CallbackTimeout); err != nil {
			m.onCallbackError(h.plugin, err)
		}
	}) {
		m.logf("dropped key %q (plugin queue full or runtime stopping)", spec)
		return false
	}
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
	m.timers.close()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, state := range m.states {
		state.L.Close()
		if state.fsRoot != nil {
			_ = state.fsRoot.Close()
		}
	}
	m.states = nil
}

// isLive reports whether L belongs to a committed plugin state. Timers and
// viewport callbacks can outlive a rolled-back startup: a ticker may already
// have queued a callback, and a viewport's on_press closure is held by the UI
// outside the registries rollbackRegistrations cleans. Both call safeCall on L,
// which panics in gopher-lua once L is closed, so those callbacks consult isLive
// before touching L. A rolled-back state is never appended to m.states.
func (m *Manager) isLive(L *lua.LState) bool {
	if m == nil || L == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.states {
		if s.L == L {
			return true
		}
	}
	return false
}

func (m *Manager) rollbackRegistrations(L *lua.LState) {
	m.events.rollbackOwner(L)
	m.commands.rollbackOwner(L)
	m.keys.rollbackOwner(L)
	m.themes.rollbackOwner(L)
	m.timers.rollbackOwner(L)
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

// openPluginFSRoot creates and opens a plugin's directory through a Root for
// the configured DataDir. Opening the child through the base Root prevents a
// pre-existing or concurrently swapped symlink from redirecting it elsewhere.
func (m *Manager) openPluginFSRoot(name string) (*os.Root, error) {
	if !filepath.IsLocal(name) || filepath.Base(name) != name || name == "." {
		return nil, fmt.Errorf("invalid plugin data directory name %q", name)
	}
	if err := os.MkdirAll(m.opts.DataDir, 0o755); err != nil {
		return nil, err
	}
	base, err := os.OpenRoot(m.opts.DataDir)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	if err := base.MkdirAll(name, 0o755); err != nil {
		return nil, err
	}
	return base.OpenRoot(name)
}

func (m *Manager) logf(format string, args ...any) {
	if m.opts.Log == nil {
		return
	}
	line := fmt.Sprintf("%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
	m.logMu.Lock()
	defer m.logMu.Unlock()
	_, _ = io.WriteString(m.opts.Log, line)
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
	name          string
	path          string
	configContext bool
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
