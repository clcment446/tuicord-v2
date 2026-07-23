package ui

import (
	"strings"

	"awesomeProject/internal/tui/widget"
)

func borderCharsForStyle(name string) widget.BorderChars {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "rounded", "":
		return widget.BorderChars{TopLeft: "╭", TopRight: "╮", BottomLeft: "╰", BottomRight: "╯", Horizontal: "─", Vertical: "│"}
	case "square":
		return widget.BorderChars{TopLeft: "┌", TopRight: "┐", BottomLeft: "└", BottomRight: "┘", Horizontal: "─", Vertical: "│"}
	case "heavy":
		return widget.BorderChars{TopLeft: "┏", TopRight: "┓", BottomLeft: "┗", BottomRight: "┛", Horizontal: "━", Vertical: "┃"}
	case "double":
		return widget.BorderChars{TopLeft: "╔", TopRight: "╗", BottomLeft: "╚", BottomRight: "╝", Horizontal: "═", Vertical: "║"}
	case "ascii":
		return widget.BorderChars{TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+", Horizontal: "-", Vertical: "|"}
	default:
		return borderCharsForStyle("rounded")
	}
}
