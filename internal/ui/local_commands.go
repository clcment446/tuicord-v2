package ui

import "strings"

type localCommand struct {
	name string
	args []string
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
