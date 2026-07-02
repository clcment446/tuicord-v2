package tui

import (
	"testing"

	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
)

func TestPostRunsInFIFOOrder(t *testing.T) {
	app := New()
	var got []int
	app.Post(func() { got = append(got, 1) })
	app.Post(func() { got = append(got, 2) })
	app.Post(func() {
		got = append(got, 3)
		app.Post(func() { got = append(got, 4) })
	})

	app.drainPosts()
	want := []int{1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("posts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("posts = %v, want %v", got, want)
		}
	}
	if !app.Dirty() {
		t.Fatal("Post drain did not invalidate")
	}
}

func TestRenderDrawsParentBeforeChildren(t *testing.T) {
	child := &drawWidget{
		testWidget: *newTestWidget("child", false),
		content:    "c",
	}
	child.node.Grow = 1
	parent := &drawWidget{
		testWidget: *newTestWidget("parent", false),
		content:    "p",
	}
	parent.node = &layout.Node{Children: []*layout.Node{child.node}}
	parent.children = []Widget{child}

	buf := New().Render(parent, Size{W: 1, H: 1})
	if got := buf.Cell(0, 0).Content; got != "c" {
		t.Fatalf("rendered cell = %q, want child drawn over parent", got)
	}
}

type drawWidget struct {
	testWidget
	content string
}

func (w *drawWidget) Draw(r screen.Region) {
	r.Set(0, 0, screen.Cell{Content: w.content})
}
