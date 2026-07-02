// Package screen stores terminal cells and emits ANSI frame diffs.
//
// A Buffer is a fixed-size grid of display cells. Each visible cell stores one
// grapheme cluster, never a rune, so combining marks and emoji presentation
// selectors stay attached to the glyph the terminal will draw. Wide clusters
// occupy two cells: the left cell carries content and the right cell is an
// internal continuation marker that the diff emitter skips.
package screen

// Rect is a rectangle in terminal cells.
type Rect struct {
	// X is the left column.
	X int
	// Y is the top row.
	Y int
	// W is the width in cells.
	W int
	// H is the height in cells.
	H int
}

// Color is an RGB terminal color. The zero value means "default color".
type Color struct {
	R, G, B uint8
	set     bool
}

// RGB returns a set 24-bit color.
func RGB(r, g, b uint8) Color {
	return Color{R: r, G: g, B: b, set: true}
}

// Set reports whether c is an explicit color rather than terminal default.
func (c Color) Set() bool {
	return c.set
}

// Attr is a bitset of text attributes.
type Attr uint16

const (
	// Bold draws bold or bright text.
	Bold Attr = 1 << iota
	// Dim draws dim text.
	Dim
	// Italic draws italic text.
	Italic
	// Underline draws underlined text.
	Underline
	// Reverse swaps foreground and background.
	Reverse
	// Strike draws struck-through text.
	Strike
)

// Style is the visual style for a cell.
type Style struct {
	// Fg is the foreground color. The zero value uses terminal default.
	Fg Color
	// Bg is the background color. The zero value uses terminal default.
	Bg Color
	// Attrs are SGR text attributes.
	Attrs Attr
}

// Cell is one terminal cell. Content should be one grapheme cluster; Set
// normalizes wider strings to their first cluster.
type Cell struct {
	// Content is the grapheme cluster drawn in this cell.
	Content string
	// Wide reports that Content occupies this cell and the next one.
	Wide bool
	// Style is the cell's visual style.
	Style Style

	continuation bool
}

// Blank is the default empty cell.
var Blank = Cell{Content: " "}
