package tui

import (
	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
)

// Event is a decoded terminal input event.
type Event = input.Event

// Size is a terminal area measured in display cells.
type Size struct {
	// W is the width in cells.
	W int
	// H is the height in cells.
	H int
}

// Rect is a rectangle measured in terminal cells.
type Rect = layout.Rect

// Widget is the minimal contract implemented by TUI components.
//
// Concrete widgets live in higher packages. Runtime code assumes Layout
// returns a stable node pointer for the current widget state so layout results
// can be associated back to widgets for hit testing and drawing.
type Widget interface {
	// Measure returns the widget's preferred size for an available area.
	Measure(avail Size) Size
	// Layout returns the layout node owned by this widget.
	Layout() *layout.Node
	// Draw paints the widget into the supplied clipped screen region.
	Draw(screen.Region)
	// Handle receives an input event and reports whether it consumed it.
	Handle(Event) bool
}

// Container is implemented by widgets that expose retained child widgets.
//
// It is intentionally separate from Widget so leaf widgets keep the small core
// contract. Children are ordered from back to front for drawing and hit tests.
type Container interface {
	// Children returns this widget's retained child widgets.
	Children() []Widget
}

// Focusable is implemented by widgets that can receive keyboard focus.
type Focusable interface {
	// CanFocus reports whether the widget should be present in the focus ring.
	CanFocus() bool
}
