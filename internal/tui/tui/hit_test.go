package tui

import (
	"testing"

	"awesomeProject/internal/tui/layout"
)

func TestBuildHitIndexReturnsDeepestWidget(t *testing.T) {
	left := newTestWidget("left", false)
	left.node.Basis = 5
	right := newTestWidget("right", false)
	right.node.Grow = 1
	root := newTestWidget("root", false)
	root.node = &layout.Node{
		Dir:      layout.Row,
		Gap:      1,
		Children: []*layout.Node{left.node, right.node},
	}
	root.children = []Widget{left, right}

	hits := BuildHitIndex(root, Size{W: 11, H: 3})
	hit, ok := hits.Hit(6, 1)
	if !ok {
		t.Fatal("Hit() did not find right widget")
	}
	if !sameWidget(hit.Widget, right) {
		t.Fatalf("Hit() = %v, want right", hit.Widget)
	}
	if hit.X != 0 || hit.Y != 1 {
		t.Fatalf("local point = (%d,%d), want (0,1)", hit.X, hit.Y)
	}

	path := hits.Path(6, 1)
	if len(path) != 2 {
		t.Fatalf("Path() length = %d, want 2", len(path))
	}
	if !sameWidget(path[0].Widget, root) || !sameWidget(path[1].Widget, right) {
		t.Fatalf("Path() = %#v, want root -> right", path)
	}
}

func TestHitIndexTieBreaksLaterEntryAtSameDepth(t *testing.T) {
	back := newTestWidget("back", false)
	front := newTestWidget("front", false)
	var hits HitIndex
	hits.Add(back, Rect{W: 5, H: 5}, 1)
	hits.Add(front, Rect{W: 5, H: 5}, 1)

	hit, ok := hits.Hit(2, 2)
	if !ok {
		t.Fatal("Hit() did not find overlapping widgets")
	}
	if !sameWidget(hit.Widget, front) {
		t.Fatalf("Hit() = %v, want front", hit.Widget)
	}
}
