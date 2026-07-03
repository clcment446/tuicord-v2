package screen

import "awesomeProject/internal/tui/text"

// Buffer is a fixed-size grid of terminal cells.
type Buffer struct {
	w, h     int
	cells    []Cell
	graphics []Graphic
}

// NewBuffer returns a blank buffer of w by h cells.
func NewBuffer(w, h int) *Buffer {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	b := &Buffer{w: w, h: h, cells: make([]Cell, w*h)}
	b.Clear()
	return b
}

// Width returns the buffer width in cells.
func (b *Buffer) Width() int {
	if b == nil {
		return 0
	}
	return b.w
}

// Height returns the buffer height in cells.
func (b *Buffer) Height() int {
	if b == nil {
		return 0
	}
	return b.h
}

// Bounds returns the rectangle covered by b.
func (b *Buffer) Bounds() Rect {
	if b == nil {
		return Rect{}
	}
	return Rect{W: b.w, H: b.h}
}

// Clear fills the buffer with blank cells.
func (b *Buffer) Clear() {
	if b == nil {
		return
	}
	for i := range b.cells {
		b.cells[i] = Blank
	}
	b.graphics = b.graphics[:0]
}

// Cell returns the cell at x,y. Out-of-bounds reads return Blank.
func (b *Buffer) Cell(x, y int) Cell {
	if b == nil || !b.inside(x, y) {
		return Blank
	}
	return b.cells[b.index(x, y)]
}

// Set writes c at x,y. Wide cells that do not fit are replaced with a blank,
// avoiding half-glyphs at the right edge.
func (b *Buffer) Set(x, y int, c Cell) {
	if b == nil || !b.inside(x, y) {
		return
	}
	c = normalizeCell(c)
	if c.Wide && x+1 >= b.w {
		c = Blank
	}

	b.clearCell(x, y)
	if c.Wide {
		b.clearCell(x+1, y)
	}
	i := b.index(x, y)
	b.cells[i] = c
	if c.Wide {
		right := Blank
		right.Style = c.Style
		right.continuation = true
		b.cells[i+1] = right
	}
}

// Fill writes c into every cell in r clipped to the buffer bounds.
func (b *Buffer) Fill(r Rect, c Cell) {
	if b == nil {
		return
	}
	r = intersect(r, b.Bounds())
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			b.Set(x, y, c)
		}
	}
}

// Clip returns a region that draws into r clipped to the buffer bounds.
func (b *Buffer) Clip(r Rect) Region {
	return b.ClipWithin(r, r)
}

// ClipWithin returns a region with local coordinates rooted at r and drawing
// clipped to clip. This is useful for retained child widgets that are laid out
// outside a scroll viewport but must draw only through the viewport window.
func (b *Buffer) ClipWithin(r, clip Rect) Region {
	if b == nil {
		return Region{}
	}
	return Region{
		buf:    b,
		origin: r,
		clip:   intersect(intersect(r, clip), b.Bounds()),
	}
}

// AddGraphic attaches terminal protocol output to this frame.
func (b *Buffer) AddGraphic(g Graphic) {
	if b == nil || g.Key == "" || len(g.Data) == 0 {
		return
	}
	g.Rect = intersect(g.Rect, b.Bounds())
	if g.Rect.W <= 0 || g.Rect.H <= 0 {
		return
	}
	b.graphics = append(b.graphics, cloneGraphic(g))
}

// Graphics returns a copy of terminal protocol output attached to this frame.
func (b *Buffer) Graphics() []Graphic {
	if b == nil || len(b.graphics) == 0 {
		return nil
	}
	out := make([]Graphic, len(b.graphics))
	for i, g := range b.graphics {
		out[i] = cloneGraphic(g)
	}
	return out
}

func (b *Buffer) inside(x, y int) bool {
	return x >= 0 && y >= 0 && x < b.w && y < b.h
}

func (b *Buffer) index(x, y int) int {
	return y*b.w + x
}

func (b *Buffer) clearCell(x, y int) {
	if !b.inside(x, y) {
		return
	}
	i := b.index(x, y)
	cell := b.cells[i]
	if cell.continuation && x > 0 {
		left := b.index(x-1, y)
		if b.cells[left].Wide {
			b.cells[left] = Blank
		}
	}
	if cell.Wide && x+1 < b.w {
		b.cells[i+1] = Blank
	}
	b.cells[i] = Blank
}

func normalizeCell(c Cell) Cell {
	if c.Content == "" {
		c.Content = " "
	}
	for cluster := range text.Clusters(c.Content) {
		c.Content = cluster.Text
		c.Wide = cluster.Width == 2
		c.continuation = false
		if cluster.Width == 0 {
			c.Content = " "
			c.Wide = false
		}
		return c
	}
	c.Content = " "
	c.Wide = false
	c.continuation = false
	return c
}

func cloneGraphic(g Graphic) Graphic {
	g.Clear = append([]byte(nil), g.Clear...)
	g.Free = append([]byte(nil), g.Free...)
	g.Upload = append([]byte(nil), g.Upload...)
	g.Data = append([]byte(nil), g.Data...)
	return g
}

func intersect(a, b Rect) Rect {
	x1 := max(a.X, b.X)
	y1 := max(a.Y, b.Y)
	x2 := min(a.X+a.W, b.X+b.W)
	y2 := min(a.Y+a.H, b.Y+b.H)
	if x2 <= x1 || y2 <= y1 {
		return Rect{X: x1, Y: y1}
	}
	return Rect{X: x1, Y: y1, W: x2 - x1, H: y2 - y1}
}
