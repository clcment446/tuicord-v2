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
		{cfg.Keys.Picker, "Open emoji / sticker picker"},
		{cfg.Keys.PasteImage, "Attach image from clipboard (or ;paste)"},
		{cfg.Keys.Help, "Toggle this help"},
		{cfg.Keys.NextPanel, "Cycle focus between panels"},
		{cfg.Keys.FocusComposer, "Return focus to the composer / close overlays"},
		{cfg.Keys.VideoPause, "Video: pause / resume"},
		{cfg.Keys.VideoSeekBackward + " / " + cfg.Keys.VideoSeekForward, "Video: seek -/+ 5 seconds"},
		{cfg.Keys.VideoReplay, "Video: replay"},
		{"↑ / ↓", "Move between rows and message stops"},
		{"0-9", "Activate a control in the focused message"},
		{"Alt+← / Alt+→", "Back/forward through visited panes"},
		{"Enter", "Send message / confirm"},
	}
	if cfg.Accessibility.VimNavigation {
		lines = append(lines,
			[2]string{"j / k", "Scroll down/up"},
			[2]string{"J / K", "Move down/up between message stops"},
			[2]string{"h / l", "Previous/next component or panel"},
			[2]string{"H / L", "Previous/next panel"},
			[2]string{"← / →", "Previous/next component"},
			[2]string{"PgUp / PgDn", "Scroll by one viewport"},
			[2]string{"-", "Toggle the focused message section"},
			[2]string{"U", "Open the focused author's profile"},
			[2]string{"V / Y", "Select message anchors / copy formatted selection"},
			[2]string{"i / ;q", "Enter / leave composer input mode"},
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
