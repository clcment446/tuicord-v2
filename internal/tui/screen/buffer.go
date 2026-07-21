package screen

import "awesomeProject/internal/tui/text"

// Buffer is a fixed-size grid of terminal cells.
type Buffer struct {
	w, h     int
	cells    []Cell
	graphics []Graphic

	// layer is the current draw layer; cellLayer stamps which layer last wrote
	// each cell. Higher layers (overlays) drawn over a graphic occlude it — see
	// SetLayer and visibleGraphics.
	layer     int
	cellLayer []uint8
}

// NewBuffer returns a blank buffer of w by h cells.
func NewBuffer(w, h int) *Buffer {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	b := &Buffer{w: w, h: h, cells: make([]Cell, w*h), cellLayer: make([]uint8, w*h)}
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
	for i := range b.cellLayer {
		b.cellLayer[i] = 0
	}
	b.graphics = b.graphics[:0]
	b.layer = 0
}

// SetLayer sets the draw layer stamped onto every subsequent cell write and
// graphic. The retained widget tree draws at layer 0; overlays (popups, toasts)
// are drawn at a higher layer so their cells occlude graphics beneath them.
// Clear resets the layer to 0.
func (b *Buffer) SetLayer(n int) {
	if b == nil {
		return
	}
	if n < 0 {
		n = 0
	}
	b.layer = n
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
	b.cellLayer[i] = uint8(b.layer)
	if c.Wide {
		right := Blank
		right.Style = c.Style
		right.continuation = true
		b.cells[i+1] = right
		b.cellLayer[i+1] = uint8(b.layer)
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
	g.layer = b.layer
	b.graphics = append(b.graphics, cloneGraphic(g))
}

// resolveGraphics returns the graphics as they should be drawn this frame after
// occlusion. A graphic is occluded where a strictly-higher draw layer (an
// overlay) painted over its cells. Strictly-higher matters — the graphic's own
// layer cells and later same-layer text (the next chat line, the ▶ glyph) must
// not occlude it. For each graphic:
//   - not covered: emitted unchanged.
//   - fully covered: dropped (its Clear is emitted by the diff).
//   - partially covered with a Reclip: re-placed over the visible sub-rectangles
//     (image shows around the overlay instead of graying out entirely).
//   - partially covered without a Reclip (e.g. sixel): dropped.
func (b *Buffer) resolveGraphics() []Graphic {
	if b == nil || len(b.graphics) == 0 {
		return nil
	}
	out := make([]Graphic, 0, len(b.graphics))
	for _, g := range b.graphics {
		hole, covered := b.occluderBBox(g)
		if !covered {
			out = append(out, cloneGraphic(g))
			continue
		}
		if g.Reclip == nil {
			continue // whole-placement suppression fallback
		}
		visible := subtractRect(intersect(g.Rect, b.Bounds()), hole)
		if len(visible) == 0 {
			continue // fully covered
		}
		resolved := cloneGraphic(g)
		resolved.Data = g.Reclip(visible)
		if len(resolved.ClearAll) > 0 {
			resolved.Clear = append([]byte(nil), resolved.ClearAll...)
		}
		resolved.split = true
		out = append(out, resolved)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// occluderBBox returns the bounding rectangle of the cells within g.Rect that a
// strictly-higher layer painted, and whether any such cell exists.
func (b *Buffer) occluderBBox(g Graphic) (Rect, bool) {
	r := intersect(g.Rect, b.Bounds())
	minX, minY := r.X+r.W, r.Y+r.H
	maxX, maxY := r.X, r.Y
	found := false
	for y := r.Y; y < r.Y+r.H; y++ {
		row := y * b.w
		for x := r.X; x < r.X+r.W; x++ {
			if int(b.cellLayer[row+x]) <= g.layer {
				continue
			}
			found = true
			minX, minY = min(minX, x), min(minY, y)
			maxX, maxY = max(maxX, x+1), max(maxY, y+1)
		}
	}
	if !found {
		return Rect{}, false
	}
	return Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}, true
}

// subtractRect returns the rectangles covering a but not hole (hole is clamped
// to a). It yields up to four rects: the full-width strips above and below hole,
// and the left and right strips beside it. An empty result means a is fully
// covered.
func subtractRect(a, hole Rect) []Rect {
	hole = intersect(a, hole)
	if hole.W <= 0 || hole.H <= 0 {
		return []Rect{a}
	}
	var out []Rect
	if hole.Y > a.Y { // top
		out = append(out, Rect{X: a.X, Y: a.Y, W: a.W, H: hole.Y - a.Y})
	}
	if bottom := hole.Y + hole.H; bottom < a.Y+a.H { // bottom
		out = append(out, Rect{X: a.X, Y: bottom, W: a.W, H: a.Y + a.H - bottom})
	}
	if hole.X > a.X { // left
		out = append(out, Rect{X: a.X, Y: hole.Y, W: hole.X - a.X, H: hole.H})
	}
	if right := hole.X + hole.W; right < a.X+a.W { // right
		out = append(out, Rect{X: right, Y: hole.Y, W: a.X + a.W - right, H: hole.H})
	}
	return out
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
			b.cellLayer[left] = uint8(b.layer)
		}
	}
	if cell.Wide && x+1 < b.w {
		b.cells[i+1] = Blank
		b.cellLayer[i+1] = uint8(b.layer)
	}
	b.cells[i] = Blank
	b.cellLayer[i] = uint8(b.layer)
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
	g.ClearAll = append([]byte(nil), g.ClearAll...)
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
