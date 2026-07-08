package widget

import (
	"strings"
	"testing"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

type fakeScroll struct {
	offset, viewport, content int
}

func (m *fakeScroll) ScrollExtent() (int, int, int) { return m.offset, m.viewport, m.content }
func (m *fakeScroll) ScrollTo(offset int) {
	m.offset = clampInt(offset, 0, maxInt(m.content-m.viewport, 0))
}

func drawBar(t *testing.T, bar *Scrollbar, height int) string {
	t.Helper()
	buf := screen.NewBuffer(1, height)
	bar.Draw(buf.Clip(buf.Bounds()))
	var b strings.Builder
	for y := range height {
		b.WriteString(buf.Cell(0, y).Content)
	}
	return b.String()
}

func TestScrollbarDrawsProportionalThumb(t *testing.T) {
	model := &fakeScroll{viewport: 10, content: 40}
	bar := NewScrollbar(model)

	if got, want := drawBar(t, bar, 8), "██░░░░░░"; got != want {
		t.Fatalf("thumb at top = %q, want %q", got, want)
	}
	model.offset = 30
	if got, want := drawBar(t, bar, 8), "░░░░░░██"; got != want {
		t.Fatalf("thumb at bottom = %q, want %q", got, want)
	}
	model.offset = 15
	if got := drawBar(t, bar, 8); strings.Count(got, "█") != 2 || strings.HasPrefix(got, "█") || strings.HasSuffix(got, "█") {
		t.Fatalf("mid thumb = %q, want two thumb cells away from the edges", got)
	}
}

func TestScrollbarIsInertWhenContentFits(t *testing.T) {
	model := &fakeScroll{viewport: 10, content: 5}
	bar := NewScrollbar(model)

	if got, want := drawBar(t, bar, 6), "░░░░░░"; got != want {
		t.Fatalf("track = %q, want %q", got, want)
	}
	if bar.Handle(input.MouseEvent{Kind: input.MousePress, Btn: input.ButtonLeft, Y: 3}) {
		t.Fatal("click on inert scrollbar should not be handled")
	}
	if _, ok := bar.DragStart(0, 3); ok {
		t.Fatal("drag on inert scrollbar should not start")
	}
}

func TestScrollbarTrackClicksPageAndWheelScrolls(t *testing.T) {
	model := &fakeScroll{offset: 15, viewport: 10, content: 40}
	bar := NewScrollbar(model)
	drawBar(t, bar, 8)

	if !bar.Handle(input.MouseEvent{Kind: input.MousePress, Btn: input.ButtonLeft, Y: 7}) {
		t.Fatal("click below thumb was not handled")
	}
	if model.offset != 25 {
		t.Fatalf("page down offset = %d, want 25", model.offset)
	}
	if !bar.Handle(input.MouseEvent{Kind: input.MousePress, Btn: input.ButtonLeft, Y: 0}) {
		t.Fatal("click above thumb was not handled")
	}
	if model.offset != 15 {
		t.Fatalf("page up offset = %d, want 15", model.offset)
	}
	if !bar.Handle(input.MouseEvent{Kind: input.MouseWheel, Btn: input.ButtonWheelDown}) {
		t.Fatal("wheel down was not handled")
	}
	if model.offset != 16 {
		t.Fatalf("wheel offset = %d, want 16", model.offset)
	}
}

func TestScrollbarThumbDragScrollsAndCancelRestores(t *testing.T) {
	model := &fakeScroll{viewport: 10, content: 40}
	bar := NewScrollbar(model)
	drawBar(t, bar, 8)

	op, ok := bar.DragStart(0, 0)
	if !ok {
		t.Fatal("drag on thumb did not start")
	}
	op.DragMove(0, 6)
	if model.offset != 30 {
		t.Fatalf("dragged offset = %d, want 30 (thumb at bottom)", model.offset)
	}
	op.DragMove(0, 3)
	if model.offset != 15 {
		t.Fatalf("dragged offset = %d, want 15 (thumb mid-track)", model.offset)
	}
	op.DragEnd(false)
	if model.offset != 0 {
		t.Fatalf("cancelled drag offset = %d, want 0", model.offset)
	}
}

func TestViewportImplementsScrollModel(t *testing.T) {
	var _ ScrollModel = (*Viewport)(nil)
	vp := NewViewport()
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line"
	}
	vp.SetLines(lines)
	buf := screen.NewBuffer(10, 10)
	vp.Draw(buf.Clip(buf.Bounds()))

	offset, viewport, content := vp.ScrollExtent()
	if offset != 0 || viewport != 10 || content != 30 {
		t.Fatalf("extent = %d,%d,%d, want 0,10,30", offset, viewport, content)
	}
	vp.ScrollTo(12)
	if _, y := vp.Scroll(); y != 12 {
		t.Fatalf("scroll y = %d, want 12", y)
	}
}

func TestViewportScrollExtentMeasuresChild(t *testing.T) {
	vp := NewViewport()
	vp.SetChild(NewText(strings.Repeat("line\n", 19) + "line"))
	buf := screen.NewBuffer(10, 5)
	vp.Draw(buf.Clip(buf.Bounds()))

	if _, _, content := vp.ScrollExtent(); content != 20 {
		t.Fatalf("child content = %d, want 20", content)
	}
}

func TestItemListImplementsScrollModel(t *testing.T) {
	var _ ScrollModel = (*ItemList)(nil)
	items := make([]Item, 30)
	for i := range items {
		items[i] = Item{Label: "row"}
	}
	list := NewItemList(items)
	buf := screen.NewBuffer(10, 10)
	list.Draw(buf.Clip(buf.Bounds()))

	if offset, viewport, content := list.ScrollExtent(); offset != 0 || viewport != 10 || content != 30 {
		t.Fatalf("extent = %d,%d,%d, want 0,10,30", offset, viewport, content)
	}
	list.ScrollTo(15)
	if offset, _, _ := list.ScrollExtent(); offset != 15 {
		t.Fatalf("offset = %d, want 15", offset)
	}
	if got := list.Selected(); got < 15 || got > 24 {
		t.Fatalf("selection = %d, want inside visible window [15,24]", got)
	}
	// The next draw must not snap back to the old selection.
	list.Draw(buf.Clip(buf.Bounds()))
	if offset, _, _ := list.ScrollExtent(); offset != 15 {
		t.Fatalf("offset after redraw = %d, want 15", offset)
	}
}

func TestScrollbarMeasureAndLayout(t *testing.T) {
	bar := NewScrollbar(nil)
	if got := bar.Measure(tui.Size{W: 20, H: 12}); got.W != 1 || got.H != 12 {
		t.Fatalf("measure = %+v, want 1x12", got)
	}
	if node := bar.Layout(); node == nil || node.Max != 1 {
		t.Fatalf("layout node = %+v, want one-cell column", node)
	}
}
