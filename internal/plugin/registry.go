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

// commandRegistry maps ;-command names to ownership stacks. Unlike the event
// bus, it is read from the UI goroutine (when the composer submits a command)
// while written on the plugin goroutine (during load), so it is mutex-guarded.
// Keeping shadowed registrations is what makes failed startup transactional: a
// rollback removes the failed state's entries and reveals the previous owner.
type commandRegistry struct {
	mu       sync.RWMutex
	commands map[string][]handler
}

func newCommandRegistry() *commandRegistry {
	return &commandRegistry{commands: make(map[string][]handler)}
}

// add registers name; a later registration of the same name wins (last loaded).
func (r *commandRegistry) add(name string, h handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[name] = append(r.commands[name], h)
}

// lookup returns the active (most recently registered) handler for name.
func (r *commandRegistry) lookup(name string) (handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	stack := r.commands[name]
	if len(stack) == 0 {
		return handler{}, false
	}
	return stack[len(stack)-1], true
}

// rollbackOwner removes every registration owned by L, restoring any shadowed
// handler below it. It runs before the failed LState is closed.
func (r *commandRegistry) rollbackOwner(L *lua.LState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, stack := range r.commands {
		stack = handlersWithoutOwner(stack, L)
		if len(stack) == 0 {
			delete(r.commands, name)
		} else {
			r.commands[name] = stack
		}
	}
}

func handlersWithoutOwner(stack []handler, L *lua.LState) []handler {
	kept := stack[:0]
	for _, h := range stack {
		if h.L != L {
			kept = append(kept, h)
		}
	}
	for i := len(kept); i < len(stack); i++ {
		stack[i] = handler{}
	}
	return kept
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
	for name, stack := range r.commands {
		if len(stack) > 0 {
			entries = append(entries, entry{Name: name, Help: stack[len(stack)-1].help})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}
