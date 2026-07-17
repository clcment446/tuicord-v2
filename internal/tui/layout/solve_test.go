package layout

import "testing"

func TestSolveRowGrow(t *testing.T) {
	left := &Node{Basis: 10}
	main := &Node{Grow: 1, Min: 5}
	right := &Node{Basis: 5}
	root := &Node{Dir: Row, Gap: 1, Children: []*Node{left, main, right}}

	rects := Solve(root, 30, 10)
	assertRect(t, rects[left], Rect{X: 0, Y: 0, W: 10, H: 10})
	assertRect(t, rects[main], Rect{X: 11, Y: 0, W: 13, H: 10})
	assertRect(t, rects[right], Rect{X: 25, Y: 0, W: 5, H: 10})
}

func TestSolveColumnPadding(t *testing.T) {
	top := &Node{Basis: 2}
	bottom := &Node{Grow: 1}
	root := &Node{
		Dir:      Column,
		Gap:      1,
		Padding:  Insets{Top: 1, Right: 2, Bottom: 1, Left: 2},
		Children: []*Node{top, bottom},
	}

	rects := Solve(root, 20, 10)
	assertRect(t, rects[root], Rect{W: 20, H: 10})
	assertRect(t, rects[top], Rect{X: 2, Y: 1, W: 16, H: 2})
	assertRect(t, rects[bottom], Rect{X: 2, Y: 4, W: 16, H: 5})
}

func TestSolveMinMax(t *testing.T) {
	a := &Node{Grow: 1, Min: 5, Max: 6}
	b := &Node{Grow: 1, Min: 5}
	root := &Node{Dir: Row, Children: []*Node{a, b}}

	rects := Solve(root, 20, 3)
	assertRect(t, rects[a], Rect{W: 6, H: 3})
	assertRect(t, rects[b], Rect{X: 6, W: 14, H: 3})
}

func TestSolveShrinkToMin(t *testing.T) {
	a := &Node{Basis: 10, Min: 4}
	b := &Node{Basis: 10, Min: 4}
	root := &Node{Dir: Row, Gap: 1, Children: []*Node{a, b}}

	rects := Solve(root, 12, 2)
	assertRect(t, rects[a], Rect{W: 5, H: 2})
	assertRect(t, rects[b], Rect{X: 6, W: 6, H: 2})
}

func TestSolveHideBelow(t *testing.T) {
	sidebar := &Node{Basis: 20, HideBelow: 100}
	main := &Node{Grow: 1}
	root := &Node{Dir: Row, Gap: 1, Children: []*Node{sidebar, main}}

	narrow := Solve(root, 80, 20)
	if _, ok := narrow[sidebar]; ok {
		t.Fatal("hidden sidebar has a rect")
	}
	assertRect(t, narrow[main], Rect{W: 80, H: 20})

	wide := Solve(root, 120, 20)
	assertRect(t, wide[sidebar], Rect{W: 20, H: 20})
	assertRect(t, wide[main], Rect{X: 21, W: 99, H: 20})
}

func TestSolveHiddenNode(t *testing.T) {
	hidden := &Node{Basis: 20, Hidden: true}
	main := &Node{Grow: 1}
	root := &Node{Dir: Row, Gap: 1, Children: []*Node{hidden, main}}
	rects := Solve(root, 40, 5)
	if _, ok := rects[hidden]; ok {
		t.Fatal("explicitly hidden node has a rect")
	}
	assertRect(t, rects[main], Rect{W: 40, H: 5})
}

func TestChildrenNeverExceedParent(t *testing.T) {
	a := &Node{Basis: 100, Min: 20}
	b := &Node{Grow: 1, Min: 20}
	root := &Node{Dir: Row, Gap: 2, Padding: Insets{Left: 1, Right: 1}, Children: []*Node{a, b}}
	rects := Solve(root, 30, 5)
	parent := rects[root]
	for _, child := range []*Node{a, b} {
		r := rects[child]
		if r.X < parent.X || r.Y < parent.Y || r.X+r.W > parent.X+parent.W || r.Y+r.H > parent.Y+parent.H {
			t.Fatalf("child %+v exceeds parent %+v", r, parent)
		}
	}
}

func assertRect(t *testing.T, got, want Rect) {
	t.Helper()
	if got != want {
		t.Fatalf("rect = %+v, want %+v", got, want)
	}
}
