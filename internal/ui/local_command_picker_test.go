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

func TestLocalCommandPickerTabAutocompletes(t *testing.T) {
	var picked string
	p := NewLocalCommandPicker([]localCommandSpec{
		{Name: "signin", Description: "Add another account"},
		{Name: "settings", Description: "Open settings"},
	}, Styles{}, "sig", func(name string) { picked = name }, func() {})
	p.Handle(input.KeyEvent{Key: input.KeyTab})
	if picked != "signin" {
		t.Fatalf("Tab autocompleted %q, want signin", picked)
	}
}
