package plugin

import "sync"

// keyRegistry maps key specs (e.g. "ctrl+g") to plugin handlers. Like the
// command registry it is read from the UI goroutine during key dispatch and
// written on the plugin goroutine during load, so it is mutex-guarded.
//
// The Shell consults this registry only after its own built-in keys, so plugin
// binds fill gaps rather than shadowing core navigation. Actual dispatch is
// wired in Phase 2 (keybindings); registration works from load time.
type keyRegistry struct {
	mu   sync.RWMutex
	keys map[string]handler
}

func newKeyRegistry() *keyRegistry {
	return &keyRegistry{keys: make(map[string]handler)}
}

func (r *keyRegistry) add(spec string, h handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys[spec] = h
}

func (r *keyRegistry) lookup(spec string) (handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.keys[spec]
	return h, ok
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
