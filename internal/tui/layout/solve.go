package layout

// Solve lays out root inside a w by h container and returns rectangles for
// every visible node. Hidden nodes are omitted from the map.
func Solve(root *Node, w, h int) map[*Node]Rect {
	rects := make(map[*Node]Rect)
	if root == nil || w <= 0 || h <= 0 {
		return rects
	}
	solve(root, Rect{W: w, H: h}, w, rects)
	return rects
}

func solve(n *Node, r Rect, rootWidth int, rects map[*Node]Rect) {
	if n == nil || hidden(n, rootWidth) || r.W <= 0 || r.H <= 0 {
		return
	}
	rects[n] = r
	children := visibleChildren(n.Children, rootWidth)
	if len(children) == 0 {
		return
	}

	inner := inset(r, n.Padding)
	if inner.W <= 0 || inner.H <= 0 {
		return
	}

	mainSize := inner.W
	crossSize := inner.H
	if n.Dir == Column {
		mainSize = inner.H
		crossSize = inner.W
	}

	gap := max(n.Gap, 0)
	totalGap := gap * max(len(children)-1, 0)
	available := max(mainSize-totalGap, 0)
	sizes := distribute(children, available)

	pos := 0
	for i, child := range children {
		main := sizes[i]
		var childRect Rect
		if n.Dir == Column {
			childRect = Rect{X: inner.X, Y: inner.Y + pos, W: crossSize, H: main}
		} else {
			childRect = Rect{X: inner.X + pos, Y: inner.Y, W: main, H: crossSize}
		}
		solve(child, childRect, rootWidth, rects)
		pos += main + gap
	}
}

func distribute(children []*Node, available int) []int {
	sizes := make([]int, len(children))
	total := 0
	totalGrow := 0.0
	for i, child := range children {
		size := clamp(child.Basis, child.Min, child.Max)
		sizes[i] = size
		total += size
		if child.Grow > 0 {
			totalGrow += child.Grow
		}
	}

	free := available - total
	if free > 0 && totalGrow > 0 {
		remaining := free
		for i, child := range children {
			if child.Grow <= 0 {
				continue
			}
			share := int(float64(free) * child.Grow / totalGrow)
			if share > remaining {
				share = remaining
			}
			next := clamp(sizes[i]+share, child.Min, child.Max)
			used := next - sizes[i]
			sizes[i] = next
			remaining -= used
		}
		for remaining > 0 {
			changed := false
			for i, child := range children {
				if remaining == 0 {
					break
				}
				if child.Grow <= 0 || atMax(sizes[i], child.Max) {
					continue
				}
				sizes[i]++
				remaining--
				changed = true
			}
			if !changed {
				break
			}
		}
		return sizes
	}

	if free < 0 {
		shrink(children, sizes, -free)
	}
	return sizes
}

func shrink(children []*Node, sizes []int, need int) {
	for need > 0 {
		changed := false
		for i, child := range children {
			if need == 0 {
				return
			}
			minSize := max(child.Min, 0)
			if sizes[i] <= minSize {
				continue
			}
			sizes[i]--
			need--
			changed = true
		}
		if !changed {
			break
		}
	}
	for need > 0 {
		changed := false
		for i := range children {
			if need == 0 {
				return
			}
			if sizes[i] == 0 {
				continue
			}
			sizes[i]--
			need--
			changed = true
		}
		if !changed {
			return
		}
	}
}

func visibleChildren(children []*Node, rootWidth int) []*Node {
	out := make([]*Node, 0, len(children))
	for _, child := range children {
		if child != nil && !hidden(child, rootWidth) {
			out = append(out, child)
		}
	}
	return out
}

func hidden(n *Node, rootWidth int) bool {
	return n.HideBelow > 0 && rootWidth < n.HideBelow
}

func inset(r Rect, p Insets) Rect {
	left := max(p.Left, 0)
	right := max(p.Right, 0)
	top := max(p.Top, 0)
	bottom := max(p.Bottom, 0)
	return Rect{
		X: r.X + left,
		Y: r.Y + top,
		W: max(r.W-left-right, 0),
		H: max(r.H-top-bottom, 0),
	}
}

func clamp(v, minSize, maxSize int) int {
	if v < minSize {
		v = minSize
	}
	if maxSize > 0 && v > maxSize {
		v = maxSize
	}
	return max(v, 0)
}

func atMax(v, maxSize int) bool {
	return maxSize > 0 && v >= maxSize
}
