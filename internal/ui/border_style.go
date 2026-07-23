package ui

import (
	"strings"

	"awesomeProject/internal/tui/widget"
)

// BorderCharsForStyle returns the configured glyph set for framed widgets.
func BorderCharsForStyle(name string) widget.BorderChars {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "rounded", "":
		return widget.BorderChars{TopLeft: "╭", TopRight: "╮", BottomLeft: "╰", BottomRight: "╯", Horizontal: "─", Vertical: "│", TeeLeft: "├", TeeRight: "┤"}
	case "square":
		return widget.BorderChars{TopLeft: "┌", TopRight: "┐", BottomLeft: "└", BottomRight: "┘", Horizontal: "─", Vertical: "│", TeeLeft: "├", TeeRight: "┤"}
	case "heavy":
		return widget.BorderChars{TopLeft: "┏", TopRight: "┓", BottomLeft: "┗", BottomRight: "┛", Horizontal: "━", Vertical: "┃", TeeLeft: "┣", TeeRight: "┫"}
	case "double":
		return widget.BorderChars{TopLeft: "╔", TopRight: "╗", BottomLeft: "╚", BottomRight: "╝", Horizontal: "═", Vertical: "║", TeeLeft: "╠", TeeRight: "╣"}
	case "ascii":
		return widget.BorderChars{TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+", Horizontal: "-", Vertical: "|", TeeLeft: "+", TeeRight: "+"}
	default:
		return BorderCharsForStyle("rounded")
	}
}

func borderCharsForStyle(name string) widget.BorderChars { return BorderCharsForStyle(name) }
