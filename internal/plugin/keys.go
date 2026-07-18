package plugin

import (
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// keyRegistry maps key specs (e.g. "ctrl+g") to plugin handlers. Like the
// command registry it is read from the UI goroutine during key dispatch and
// written on the plugin goroutine during load, so it is mutex-guarded.
//
// The Shell consults this registry only after its own built-in keys, so plugin
// binds fill gaps rather than shadowing core navigation. Actual dispatch is
// wired in Phase 2 (keybindings); registration works from load time.
type keyRegistry struct {
	mu   sync.RWMutex
	keys map[string][]handler
}

func newKeyRegistry() *keyRegistry {
	return &keyRegistry{keys: make(map[string][]handler)}
}

func (r *keyRegistry) add(spec string, h handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys[spec] = append(r.keys[spec], h)
}

func (r *keyRegistry) lookup(spec string) (handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	stack := r.keys[spec]
	if len(stack) == 0 {
		return handler{}, false
	}
	return stack[len(stack)-1], true
}

// rollbackOwner removes the failed state's registrations and reveals any
// previously shadowed key handler.
func (r *keyRegistry) rollbackOwner(L *lua.LState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for spec, stack := range r.keys {
		stack = handlersWithoutOwner(stack, L)
		if len(stack) == 0 {
			delete(r.keys, spec)
		} else {
			r.keys[spec] = stack
		}
	}
}

func (r *keyRegistry) specs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.keys))
	for spec := range r.keys {
		out = append(out, spec)
	}
	return out
}
