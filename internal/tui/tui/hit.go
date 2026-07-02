package tui

import "awesomeProject/internal/tui/layout"

// HitEntry associates a laid-out rectangle with a widget.
type HitEntry struct {
	// Widget is the widget occupying Rect.
	Widget Widget
	// Rect is the widget rectangle in root coordinates.
	Rect Rect
	// Depth is the retained tree depth, where the root is zero.
	Depth int
	// Order is the insertion order. Later entries are considered above earlier
	// entries when depths are equal.
	Order int
}

// Hit is the result of a point query against a HitIndex.
type Hit struct {
	HitEntry
	// X is the point's x coordinate relative to Rect.
	X int
	// Y is the point's y coordinate relative to Rect.
	Y int
}

// HitIndex stores rectangles from the most recent layout pass.
type HitIndex struct {
	entries []HitEntry
}

// BuildHitIndex lays out root and returns a hit-test index for visible widgets.
func BuildHitIndex(root Widget, bounds Size) HitIndex {
	var idx HitIndex
	if root == nil || bounds.W <= 0 || bounds.H <= 0 {
		return idx
	}
	rects := layout.Solve(root.Layout(), bounds.W, bounds.H)
	addHitTree(&idx, root, rects, 0)
	return idx
}

// Add inserts a widget rectangle into the index.
func (h *HitIndex) Add(w Widget, r Rect, depth int) {
	if h == nil || w == nil || r.W <= 0 || r.H <= 0 {
		return
	}
	h.entries = append(h.entries, HitEntry{
		Widget: w,
		Rect:   r,
		Depth:  depth,
		Order:  len(h.entries),
	})
}

// Entries returns a copy of the indexed rectangles.
func (h HitIndex) Entries() []HitEntry {
	out := make([]HitEntry, len(h.entries))
	copy(out, h.entries)
	return out
}

// Hit returns the topmost widget under x,y.
func (h HitIndex) Hit(x, y int) (Hit, bool) {
	path := h.Path(x, y)
	if len(path) == 0 {
		return Hit{}, false
	}
	return path[len(path)-1], true
}

// Path returns every widget containing x,y from root toward the topmost hit.
func (h HitIndex) Path(x, y int) []Hit {
	var out []Hit
	for _, entry := range h.entries {
		if !contains(entry.Rect, x, y) {
			continue
		}
		hit := Hit{
			HitEntry: entry,
			X:        x - entry.Rect.X,
			Y:        y - entry.Rect.Y,
		}
		out = append(out, hit)
	}
	sortHits(out)
	return out
}

func addHitTree(idx *HitIndex, w Widget, rects map[*layout.Node]layout.Rect, depth int) {
	if w == nil {
		return
	}
	node := w.Layout()
	if r, ok := rects[node]; ok {
		idx.Add(w, r, depth)
	}
	container, ok := w.(Container)
	if !ok {
		return
	}
	for _, child := range container.Children() {
		addHitTree(idx, child, rects, depth+1)
	}
}

func contains(r Rect, x, y int) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H
}

func sortHits(hits []Hit) {
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && lessHit(hits[j-1], hits[j]); j-- {
			hits[j-1], hits[j] = hits[j], hits[j-1]
		}
	}
}

func lessHit(a, b Hit) bool {
	if a.Depth != b.Depth {
		return a.Depth > b.Depth
	}
	return a.Order > b.Order
}
