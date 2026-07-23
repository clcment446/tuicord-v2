package widget

import (
	"strings"
)

// BorderCharsForStyle returns the configured glyph set for framed widgets.
func BorderCharsForStyle(name string) BorderChars {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "rounded", "":
		return BorderChars{TopLeft: "╭", TopRight: "╮", BottomLeft: "╰", BottomRight: "╯", Horizontal: "─", Vertical: "│", TeeLeft: "├", TeeRight: "┤"}
	case "square":
		return BorderChars{TopLeft: "┌", TopRight: "┐", BottomLeft: "└", BottomRight: "┘", Horizontal: "─", Vertical: "│", TeeLeft: "├", TeeRight: "┤"}
	case "heavy":
		return BorderChars{TopLeft: "┏", TopRight: "┓", BottomLeft: "┗", BottomRight: "┛", Horizontal: "━", Vertical: "┃", TeeLeft: "┣", TeeRight: "┫"}
	case "double":
		return BorderChars{TopLeft: "╔", TopRight: "╗", BottomLeft: "╚", BottomRight: "╝", Horizontal: "═", Vertical: "║", TeeLeft: "╠", TeeRight: "╣"}
	case "ascii":
		return BorderChars{TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+", Horizontal: "-", Vertical: "|", TeeLeft: "+", TeeRight: "+"}
	default:
		return BorderCharsForStyle("rounded")
	}
}
