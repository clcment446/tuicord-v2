package screen

// Region is a clipped drawing view into a Buffer.
type Region struct {
	buf  *Buffer
	rect Rect
}

// Bounds returns the region rectangle in buffer coordinates.
func (r Region) Bounds() Rect {
	return r.rect
}

// Width returns the region width in cells.
func (r Region) Width() int {
	return r.rect.W
}

// Height returns the region height in cells.
func (r Region) Height() int {
	return r.rect.H
}

// Set writes c at region-local x,y.
func (r Region) Set(x, y int, c Cell) {
	if x < 0 || y < 0 || x >= r.rect.W || y >= r.rect.H {
		return
	}
	r.buf.Set(r.rect.X+x, r.rect.Y+y, c)
}

// Fill writes c into rect clipped to this region.
func (r Region) Fill(rect Rect, c Cell) {
	rect.X += r.rect.X
	rect.Y += r.rect.Y
	r.buf.Fill(intersect(rect, r.rect), c)
}
