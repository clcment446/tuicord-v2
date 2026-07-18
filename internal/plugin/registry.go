package plugin

import (
	"sort"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// handler is a Lua callback plus the state it belongs to.
type handler struct {
	L      *lua.LState
	fn     *lua.LFunction
	plugin string
	help   string
}

// commandRegistry maps ;-command names to plugin handlers. Unlike the event
// bus, it is read from the UI goroutine (when the composer submits a command)
// while written on the plugin goroutine (during load), so it is mutex-guarded.
type commandRegistry struct {
	mu       sync.RWMutex
	commands map[string]handler
}

func newCommandRegistry() *commandRegistry {
	return &commandRegistry{commands: make(map[string]handler)}
}

// add registers name; a later registration of the same name wins (last loaded).
func (r *commandRegistry) add(name string, h handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[name] = h
}

// lookup returns the handler for name, if registered.
func (r *commandRegistry) lookup(name string) (handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.commands[name]
	return h, ok
}

func (r *commandRegistry) removeState(L *lua.LState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, h := range r.commands {
		if h.L == L {
			delete(r.commands, name)
		}
	}
}

// entry describes a registered command for listing in help.
type entry struct {
	Name string
	Help string
}

// list returns registered commands sorted by name, for a ;help listing.
func (r *commandRegistry) list() []entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]entry, 0, len(r.commands))
	for name, h := range r.commands {
		entries = append(entries, entry{Name: name, Help: h.help})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}
