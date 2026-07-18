package ui

import (
	"fmt"
	"strings"
)

type localCommand struct {
	name string
	args []string
}

type localCommandPlugin struct {
	Names []string
	Run   func([]string)
}

type localCommandRegistry struct {
	handlers map[string]func([]string)
}

func newLocalCommandRegistry() *localCommandRegistry {
	return &localCommandRegistry{handlers: make(map[string]func([]string))}
}

func (r *localCommandRegistry) register(plugin localCommandPlugin) error {
	if r == nil || len(plugin.Names) == 0 || plugin.Run == nil {
		return fmt.Errorf("invalid local command plugin")
	}
	aliases := make([]string, 0, len(plugin.Names))
	for _, raw := range plugin.Names {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			return fmt.Errorf("local command plugin has an empty alias")
		}
		if _, exists := r.handlers[name]; exists {
			return fmt.Errorf("local command %q is already registered", name)
		}
		aliases = append(aliases, name)
	}
	for _, name := range aliases {
		r.handlers[name] = plugin.Run
	}
	return nil
}

func (r *localCommandRegistry) run(name string, args []string) bool {
	if r == nil {
		return false
	}
	handler, ok := r.handlers[strings.ToLower(name)]
	if !ok {
		return false
	}
	handler(append([]string(nil), args...))
	return true
}

// parseLocalCommand recognizes the local-only ';' namespace. It does not
// interpret ordinary messages or Discord '/' application commands.
func parseLocalCommand(input string) (localCommand, bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, ";") {
		return localCommand{}, false
	}
	fields := strings.Fields(strings.TrimPrefix(input, ";"))
	if len(fields) == 0 {
		return localCommand{}, false
	}
	return localCommand{name: strings.ToLower(fields[0]), args: append([]string(nil), fields[1:]...)}, true
}
