package screen

// Region is a clipped drawing view into a Buffer.
type Region struct {
	buf    *Buffer
	origin Rect
	clip   Rect
}

// Bounds returns the region's local coordinate rectangle in buffer coordinates.
func (r Region) Bounds() Rect {
	return r.origin
}

// VisibleBounds returns the portion of this region that is visible in buffer
// coordinates after clipping.
func (r Region) VisibleBounds() Rect {
	return intersect(r.origin, r.clip)
}

// Width returns the region width in cells.
func (r Region) Width() int {
	return r.origin.W
}

// Height returns the region height in cells.
func (r Region) Height() int {
	return r.origin.H
}

// Set writes c at region-local x,y.
func (r Region) Set(x, y int, c Cell) {
	if r.buf == nil || x < 0 || y < 0 || x >= r.origin.W || y >= r.origin.H {
		return
	}
	px := r.origin.X + x
	py := r.origin.Y + y
	if !contains(r.clip, px, py) {
		return
	}
	r.buf.Set(px, py, c)
}

// Fill writes c into rect clipped to this region.
func (r Region) Fill(rect Rect, c Cell) {
	if r.buf == nil {
		return
	}
	rect.X += r.origin.X
	rect.Y += r.origin.Y
	r.buf.Fill(intersect(rect, r.clip), c)
}

// Clip returns a child region rooted at rect in this region's local coordinate
// space and clipped to the current visible area.
func (r Region) Clip(rect Rect) Region {
	if r.buf == nil {
		return Region{}
	}
	rect.X += r.origin.X
	rect.Y += r.origin.Y
	return Region{
		buf:    r.buf,
		origin: rect,
		clip:   intersect(rect, r.clip),
	}
}

// AddGraphic attaches terminal protocol output to this region. Graphic.Rect is
// interpreted in region-local coordinates; an empty Rect uses the whole region.
func (r Region) AddGraphic(g Graphic) {
	if r.buf == nil {
		return
	}
	rect := g.Rect
	if rect.W <= 0 || rect.H <= 0 {
		rect = Rect{W: r.origin.W, H: r.origin.H}
	}
	rect.X += r.origin.X
	rect.Y += r.origin.Y
	rect = intersect(rect, r.clip)
	if rect.W <= 0 || rect.H <= 0 {
		return
	}
	g.Rect = rect
	r.buf.AddGraphic(g)
}
