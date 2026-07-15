package ui

import (
	"awesomeProject/internal/config"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
	"awesomeProject/internal/tui/widget"
)

// NewHelpOverlay builds a read-only panel listing the client's key bindings.
func NewHelpOverlay(cfg config.Config) tui.Widget {
	lines := [][2]string{
		{cfg.Keys.QuickSwitcher, "Quick switch channels"},
		{cfg.Keys.Help, "Toggle this help"},
		{cfg.Keys.NextPanel, "Cycle focus between panels"},
		{cfg.Keys.FocusComposer, "Return focus to the composer / close overlays"},
		{"↑ / ↓", "Move between rows and message stops"},
		{"0-9", "Activate a control in the focused message"},
		{"Alt+← / Alt+→", "Back/forward through visited panes"},
		{"Enter", "Send message / confirm"},
	}
	if cfg.Accessibility.VimNavigation {
		lines = append(lines,
			[2]string{"j / k", "Move between rows and message stops"},
			[2]string{"h / l", "Previous/next panel; unfold selected group"},
			[2]string{"-", "Toggle the focused message section"},
			[2]string{"U", "Open the focused author's profile"},
			[2]string{"V / Y", "Select message anchors / copy formatted selection"},
			[2]string{"I / ;q", "Enter / leave composer input mode"},
		)
	}
	rows := make([]tui.Widget, 0, len(lines)+1)
	rows = append(rows, widget.NewText("Keyboard shortcuts"))
	for _, l := range lines {
		rows = append(rows, widget.NewText(pad(l[0], 14)+l[1]))
	}
	return titled("Help", widget.Column(rows...))
}

func pad(s string, width int) string {
	for text.Width(s) < width {
		s += " "
	}
	return s
}
