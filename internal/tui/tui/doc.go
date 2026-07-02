// Package tui provides the retained runtime layer for terminal UI widgets.
//
// The lower packages decode input, solve layout, and draw terminal cells. This
// package binds those pieces together without defining any concrete widgets:
// widgets expose layout policy, drawing, and event handling; App keeps the
// frame dirty bit, routes events, runs posted UI work in order, and owns focus,
// hit testing, and drag capture state.
package tui
