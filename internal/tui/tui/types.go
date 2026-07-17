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

// Overlay is implemented by widgets that draw transient UI above their normal
// retained subtree after every child has drawn.
type Overlay interface {
	DrawOverlay(screen.Region)
}

// EventOverlay optionally receives events before retained children. It is used
// for transient popups that are drawn over, but are not part of, the tree.
type EventOverlay interface {
	HandleOverlay(Event) bool
}

// Focusable is implemented by widgets that can receive keyboard focus.
type Focusable interface {
	// CanFocus reports whether the widget should be present in the focus ring.
	CanFocus() bool
}

// FocusConfigurable lets the runtime enable or disable optional focus targets
// such as split selectors without coupling the core to a concrete widget.
type FocusConfigurable interface {
	SetFocusEnabled(bool)
}

// FocusIndicator is implemented by widgets that visually react when keyboard
// focus is inside their retained subtree.
type FocusIndicator interface {
	SetFocused(bool)
}

// FocusOwnerIndicator is implemented by widgets whose visual state should
// reflect exact focus ownership rather than focus anywhere in their subtree.
type FocusOwnerIndicator interface {
	SetFocusOwner(bool)
}

// PreferredFocus is implemented by widgets that should receive initial focus
// when no previous focused widget can be preserved.
type PreferredFocus interface {
	PreferredFocus() bool
}

// VimFocusTraverser opts a focused widget into h/l focus traversal. The widget
// may consume the key for a local expand/unfold action; returning false asks
// the runtime to move through the normal focus ring.
type VimFocusTraverser interface {
	VimFocusEnabled() bool
	HandleVimFocus(forward bool) bool
}

// FocusRequester lets a root widget request an exact focus transfer after the
// current event has finished routing. Modal input modes use it to move between
// a navigation surface and an editor without exposing the FocusManager.
type FocusRequester interface {
	TakeFocusRequest() Widget
}
