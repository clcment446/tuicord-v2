package ui

import "testing"

func TestParseLocalCommand(t *testing.T) {
	tests := []struct {
		input string
		name  string
		args  []string
		ok    bool
	}{
		{";help switch", "help", []string{"switch"}, true},
		{" ;switch general ", "switch", []string{"general"}, true},
		{"hello", "", nil, false},
		{";", "", nil, false},
	}
	for _, tt := range tests {
		got, ok := parseLocalCommand(tt.input)
		if ok != tt.ok || got.name != tt.name || !sameStrings(got.args, tt.args) {
			t.Fatalf("parseLocalCommand(%q) = %#v, %v", tt.input, got, ok)
		}
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
