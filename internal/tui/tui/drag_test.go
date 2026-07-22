package tui

import (
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
)

func TestDragCaptureReceivesMotionOutsideStartWidget(t *testing.T) {
	op := &recordDragOp{}
	drag := &dragWidget{
		testWidget: *newTestWidget("drag", false),
		op:         op,
	}
	drag.node.Basis = 5
	other := newTestWidget("other", false)
	other.node.Grow = 1
	root := newTestWidget("root", false)
	root.node = &layout.Node{
		Dir:      layout.Row,
		Gap:      1,
		Children: []*layout.Node{drag.node, other.node},
	}
	root.children = []Widget{drag, other}

	app := New()
	app.Render(root, Size{W: 12, H: 4})
	if !app.Handle(input.MouseEvent{X: 1, Y: 1, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("press did not start drag")
	}
	if !app.Drag.Active() {
		t.Fatal("drag is not active after press")
	}
	if !app.Handle(input.MouseEvent{X: 10, Y: 3, Btn: input.ButtonLeft, Kind: input.MouseMotion}) {
		t.Fatal("motion during drag was not captured")
	}
	if other.handled != 0 {
		t.Fatalf("other widget handled %d events during capture, want 0", other.handled)
	}
	if got := op.moves; len(got) != 1 || got[0] != [2]int{9, 2} {
		t.Fatalf("moves = %#v, want [(9,2)]", got)
	}
	if !app.Handle(input.MouseEvent{X: 10, Y: 3, Btn: input.ButtonLeft, Kind: input.MouseRelease}) {
		t.Fatal("release during drag was not captured")
	}
	if !op.ended || !op.commit {
		t.Fatalf("ended=%v commit=%v, want ended commit", op.ended, op.commit)
	}
}

func TestDragCancelEndsWithoutCommit(t *testing.T) {
	op := &recordDragOp{}
	var drag DragManager
	var hits HitIndex
	w := &dragWidget{testWidget: *newTestWidget("drag", false), op: op}
	hits.Add(w, Rect{W: 5, H: 5}, 0)

	drag.HandleMouse(input.MouseEvent{X: 1, Y: 1, Btn: input.ButtonLeft, Kind: input.MousePress}, hits)
	if !drag.Cancel() {
		t.Fatal("Cancel() = false, want true")
	}
	if !op.ended || op.commit {
		t.Fatalf("ended=%v commit=%v, want ended without commit", op.ended, op.commit)
	}
}

func TestTransientOverlayOwnsDragWithoutRootGeometry(t *testing.T) {
	op := &recordDragOp{}
	target := &dragWidget{testWidget: *newTestWidget("floating", false), op: op}
	root := &overlayHitRoot{testWidget: *newTestWidget("root", false), target: target}
	app := New()
	app.Render(root, Size{W: 30, H: 10})
	if !app.Handle(input.MouseEvent{X: 12, Y: 3, Btn: input.ButtonLeft, Kind: input.MousePress}) {
		t.Fatal("transient overlay press was not handled")
	}
	if !app.Drag.Active() {
		t.Fatal("transient overlay did not start drag capture")
	}
}

type dragWidget struct {
	testWidget
	op *recordDragOp
}

type overlayHitRoot struct {
	testWidget
	target Widget
}

func (w *overlayHitRoot) OverlayAt(x, y int) Widget {
	if x == 12 && y == 3 {
		return w.target
	}
	return nil
}

func (w *dragWidget) DragStart(x, y int) (DragOp, bool) {
	if x < 0 || y < 0 {
		return nil, false
	}
	return w.op, true
}

type recordDragOp struct {
	moves  [][2]int
	ended  bool
	commit bool
}

func (op *recordDragOp) DragMove(dx, dy int) {
	op.moves = append(op.moves, [2]int{dx, dy})
}

func (op *recordDragOp) DragEnd(commit bool) {
	op.ended = true
	op.commit = commit
}
