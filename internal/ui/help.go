package ui

import (
	"awesomeProject/internal/config"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// NewHelpOverlay builds a read-only panel listing the client's key bindings.
func NewHelpOverlay(cfg config.Config) tui.Widget {
	lines := [][2]string{
		{cfg.Keys.QuickSwitcher, "Quick switch channels"},
		{cfg.Keys.Picker, "Open emoji / sticker picker"},
		{cfg.Keys.Help, "Toggle this help"},
		{cfg.Keys.NextPanel, "Cycle focus between panels"},
		{cfg.Keys.FocusComposer, "Return focus to the composer / close overlays"},
		{"↑ / ↓", "Move selection or scroll"},
		{"Enter", "Send message / confirm"},
	}
	rows := make([]tui.Widget, 0, len(lines)+1)
	rows = append(rows, widget.NewText("Keyboard shortcuts"))
	for _, l := range lines {
		rows = append(rows, widget.NewText(pad(l[0], 14)+l[1]))
	}
	return titled("Help", widget.Column(rows...))
}

func pad(s string, width int) string {
	for len(s) < width {
		s += " "
	}
	return s
}
