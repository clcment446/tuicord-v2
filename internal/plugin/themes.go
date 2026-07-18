package plugin

import (
	"sort"
	"sync"
)

// themeRegistry maps a theme name to its palette (semantic color name -> hex).
// Themes are registered on the plugin goroutine during load and applied from
// the UI goroutine via the ;theme command, so it is mutex-guarded.
type themeRegistry struct {
	mu     sync.RWMutex
	themes map[string]map[string]string
}

func newThemeRegistry() *themeRegistry {
	return &themeRegistry{themes: make(map[string]map[string]string)}
}

func (r *themeRegistry) add(name string, palette map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.themes[name] = palette
}

func (r *themeRegistry) lookup(name string) (map[string]string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.themes[name]
	return p, ok
}

func (r *themeRegistry) names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.themes))
	for name := range r.themes {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
