// Package widget provides the starter controls for the retained TUI runtime.
//
// Widgets keep state and layout policy, then draw into screen regions supplied
// by the runtime. Text-handling widgets measure and edit by grapheme cluster so
// emoji, combining marks, and other user-perceived characters stay intact.
package widget
