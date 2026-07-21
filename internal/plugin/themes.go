package plugin

import (
	"fmt"
	"sort"
	"strconv"
	"sync"

	"awesomeProject/internal/config"
	lua "github.com/yuin/gopher-lua"
)

// ownedTheme records the LState that registered a validated theme. Ownership is
// retained even though Theme contains no Lua pointers so failed startup can
// reveal a shadowed registration transactionally.
type ownedTheme struct {
	owner *lua.LState
	theme config.Theme
}

type themeRegistry struct {
	mu     sync.RWMutex
	themes map[string][]ownedTheme
}

func newThemeRegistry() *themeRegistry {
	return &themeRegistry{themes: make(map[string][]ownedTheme)}
}

func (r *themeRegistry) add(name string, theme config.Theme, owner *lua.LState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.themes[name] = append(r.themes[name], ownedTheme{owner: owner, theme: theme})
}

func (r *themeRegistry) lookup(name string) (config.Theme, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	stack := r.themes[name]
	if len(stack) == 0 {
		return config.Theme{}, false
	}
	return stack[len(stack)-1].theme, true
}

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

// decodeTheme accepts the legacy flat seven-color table and the preferred
// {palette={...}, styles={selector={...}}} form.
func decodeTheme(table *lua.LTable) (config.Theme, error) {
	palette := make(map[string]string)
	styles := make(map[string]map[string]string)
	preferred := table.RawGetString("palette") != lua.LNil || table.RawGetString("styles") != lua.LNil
	var decodeErr error
	if preferred {
		table.ForEach(func(key, value lua.LValue) {
			if decodeErr != nil {
				return
			}
			name, ok := key.(lua.LString)
			if !ok {
				decodeErr = fmt.Errorf("theme: key must be a string")
				return
			}
			switch string(name) {
			case "palette":
				child, ok := value.(*lua.LTable)
				if !ok {
					decodeErr = fmt.Errorf("theme.palette: want table, got %s", value.Type())
					return
				}
				decodeErr = decodePalette(child, palette)
			case "styles":
				child, ok := value.(*lua.LTable)
				if !ok {
					decodeErr = fmt.Errorf("theme.styles: want table, got %s", value.Type())
					return
				}
				decodeErr = decodeThemeStyles(child, styles)
			default:
				decodeErr = fmt.Errorf("theme.%s: unknown field", name)
			}
		})
	} else {
		decodeErr = decodePalette(table, palette)
	}
	if decodeErr != nil {
		return config.Theme{}, decodeErr
	}
	return config.NewTheme(palette, styles)
}

func decodePalette(table *lua.LTable, out map[string]string) error {
	var decodeErr error
	table.ForEach(func(key, value lua.LValue) {
		if decodeErr != nil {
			return
		}
		name, ok := key.(lua.LString)
		if !ok {
			decodeErr = fmt.Errorf("theme.palette: key must be a string")
			return
		}
		color, ok := value.(lua.LString)
		if !ok {
			decodeErr = fmt.Errorf("theme.palette.%s: want string, got %s", name, value.Type())
			return
		}
		out[string(name)] = string(color)
	})
	return decodeErr
}

func decodeThemeStyles(table *lua.LTable, out map[string]map[string]string) error {
	var decodeErr error
	table.ForEach(func(key, value lua.LValue) {
		if decodeErr != nil {
			return
		}
		selector, ok := key.(lua.LString)
		if !ok {
			decodeErr = fmt.Errorf("theme.styles: selector must be a string")
			return
		}
		propsTable, ok := value.(*lua.LTable)
		if !ok {
			decodeErr = fmt.Errorf("theme.styles.%s: want table, got %s", selector, value.Type())
			return
		}
		props := make(map[string]string)
		propsTable.ForEach(func(propKey, propValue lua.LValue) {
			if decodeErr != nil {
				return
			}
			property, ok := propKey.(lua.LString)
			if !ok {
				decodeErr = fmt.Errorf("theme.styles.%s: property must be a string", selector)
				return
			}
			switch v := propValue.(type) {
			case lua.LString:
				props[string(property)] = string(v)
			case lua.LBool:
				props[string(property)] = strconv.FormatBool(bool(v))
			default:
				decodeErr = fmt.Errorf("theme.styles.%s.%s: want string or boolean, got %s", selector, property, propValue.Type())
			}
		})
		out[string(selector)] = props
	})
	return decodeErr
}
