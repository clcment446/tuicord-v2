package store

// LerpColor linearly interpolates between two 0xRRGGBB colors.
// t is clamped to [0, 1]: t=0 returns a, t=1 returns b.
// Each channel is interpolated independently with rounding.
func LerpColor(a, b uint32, t float64) uint32 {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	ar := float64(a >> 16 & 0xFF)
	ag := float64(a >> 8 & 0xFF)
	ab := float64(a & 0xFF)
	br := float64(b >> 16 & 0xFF)
	bg := float64(b >> 8 & 0xFF)
	bb := float64(b & 0xFF)
	r := uint32(ar+(br-ar)*t + 0.5)
	g := uint32(ag+(bg-ag)*t + 0.5)
	bl := uint32(ab+(bb-ab)*t + 0.5)
	return r<<16 | g<<8 | bl
}

// GradientAt returns the role's display color at position t ∈ [0, 1] along
// the gradient of the name text.
//
//   - When Colors is all zero (no gradient configured), the flat Color is
//     returned for all t.
//   - When only Colors[0] (Primary) is set, Primary is returned for all t.
//   - When Primary and Secondary are set, the result is a linear interpolation
//     Primary→Secondary.
//   - When all three stops are set (holographic), the gradient is split at the
//     midpoint: Primary→Secondary for t<0.5, Secondary→Tertiary for t≥0.5.
func (r Role) GradientAt(t float64) uint32 {
	p, s, ter := r.Colors[0], r.Colors[1], r.Colors[2]
	if p == 0 && s == 0 && ter == 0 {
		return r.Color
	}
	if s == 0 {
		if p != 0 {
			return p
		}
		return r.Color
	}
	if ter == 0 {
		return LerpColor(p, s, t)
	}
	// Three-stop holographic interpolation.
	if t < 0.5 {
		return LerpColor(p, s, t*2)
	}
	return LerpColor(s, ter, (t-0.5)*2)
}
