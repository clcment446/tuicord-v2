package plugin

import (
	"sort"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// ownedTheme records the LState that registered a palette. Theme ownership is
// needed even though palettes contain no Lua pointers: failed startup must not
// leak a new theme or permanently replace a previously registered one.
type ownedTheme struct {
	owner   *lua.LState
	palette map[string]string
}

// themeRegistry maps a theme name to an ownership stack. Themes are registered
// on the plugin goroutine during load and applied from the UI goroutine via the
// ;theme command, so it is mutex-guarded. The stack transactionally restores a
// shadowed palette when the latest owner's startup fails.
type themeRegistry struct {
	mu     sync.RWMutex
	themes map[string][]ownedTheme
}

func newThemeRegistry() *themeRegistry {
	return &themeRegistry{themes: make(map[string][]ownedTheme)}
}

func (r *themeRegistry) add(name string, palette map[string]string, owner *lua.LState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.themes[name] = append(r.themes[name], ownedTheme{owner: owner, palette: palette})
}

func (r *themeRegistry) lookup(name string) (map[string]string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	stack := r.themes[name]
	if len(stack) == 0 {
		return nil, false
	}
	return stack[len(stack)-1].palette, true
}

// rollbackOwner removes all themes registered by a failed state, revealing any
// prior owner and deleting names that had no prior registration.
func (r *themeRegistry) rollbackOwner(owner *lua.LState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, stack := range r.themes {
		kept := stack[:0]
		for _, theme := range stack {
			if theme.owner != owner {
				kept = append(kept, theme)
			}
		}
		for i := len(kept); i < len(stack); i++ {
			stack[i] = ownedTheme{}
		}
		if len(kept) == 0 {
			delete(r.themes, name)
		} else {
			r.themes[name] = kept
		}
	}
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
