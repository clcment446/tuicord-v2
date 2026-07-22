package ui

import (
	"testing"

	"awesomeProject/internal/tui/input"
)

func TestLocalCommandPickerFiltersAndSelects(t *testing.T) {
	var picked string
	p := NewLocalCommandPicker([]localCommandSpec{
		{Name: "help", Description: "Show commands"},
		{Name: "settings", Description: "Open settings"},
	}, Styles{}, "hel", func(name string) { picked = name }, func() {})
	p.Handle(input.KeyEvent{Key: input.KeyEnter})
	if picked != "help" {
		t.Fatalf("picked = %q, want help", picked)
	}
}
