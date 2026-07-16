package ui

import (
	"testing"

	"awesomeProject/internal/app"
	"awesomeProject/internal/tui/input"

	"github.com/diamondburned/arikawa/v3/discord"
)

func TestCommandPickerFiltersAndSelects(t *testing.T) {
	var selected app.ApplicationCommand
	p := NewCommandPicker([]app.ApplicationCommand{
		{Command: discord.Command{ID: 1, Name: "weather", Description: "Forecast"}},
		{Command: discord.Command{ID: 2, Name: "poll", Description: "Create a poll"}},
	}, Styles{}, "wea", func(command app.ApplicationCommand) { selected = command }, func() {})
	if len(p.filtered) != 1 || p.filtered[0].Name != "weather" {
		t.Fatalf("filtered = %#v", p.filtered)
	}
	p.Handle(input.KeyEvent{Key: input.KeyEnter})
	if selected.Name != "weather" {
		t.Fatalf("selected = %#v", selected)
	}
}
