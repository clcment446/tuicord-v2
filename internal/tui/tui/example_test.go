package tui_test

import (
	"fmt"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/tui"
)

type exampleWidget struct {
	name     string
	node     *layout.Node
	children []tui.Widget
}

func (w *exampleWidget) Measure(tui.Size) tui.Size { return tui.Size{} }
func (w *exampleWidget) Layout() *layout.Node      { return w.node }
func (w *exampleWidget) Draw(screen.Region)        {}
func (w *exampleWidget) Handle(tui.Event) bool     { return false }
func (w *exampleWidget) Children() []tui.Widget    { return w.children }

func ExampleBuildHitIndex() {
	sidebar := &exampleWidget{name: "sidebar", node: &layout.Node{Basis: 12}}
	chat := &exampleWidget{name: "chat", node: &layout.Node{Grow: 1}}
	root := &exampleWidget{
		name: "root",
		node: &layout.Node{
			Dir:      layout.Row,
			Gap:      1,
			Children: []*layout.Node{sidebar.node, chat.node},
		},
		children: []tui.Widget{sidebar, chat},
	}

	hits := tui.BuildHitIndex(root, tui.Size{W: 40, H: 10})
	hit, _ := hits.Hit(20, 3)
	fmt.Println(hit.Widget.(*exampleWidget).name)
	// Output:
	// chat
}

func ExampleFocusManager() {
	first := &focusExampleWidget{exampleWidget: exampleWidget{name: "first", node: &layout.Node{}}}
	second := &focusExampleWidget{exampleWidget: exampleWidget{name: "second", node: &layout.Node{}}}

	var focus tui.FocusManager
	focus.Register(first)
	focus.Register(second)
	focus.Next()

	ev := input.KeyEvent{Key: input.KeyEnter}
	fmt.Println(focus.Focused().Handle(ev))
	// Output:
	// true
}

type focusExampleWidget struct {
	exampleWidget
}

func (w *focusExampleWidget) CanFocus() bool        { return true }
func (w *focusExampleWidget) Handle(tui.Event) bool { return true }
