---
name: alt-pane-focus-history
summary: Alt+Left/Right pane navigation belongs in the shared focus manager so all focus visits can provide back/forward history.
tags: [#tui, #focus, #keyboard, #history, #panes]
impact: normal
commit: ab0672e (dirty)
date: 2026-07-17
created_at: 2026-07-17T00:00:00+02:00
scope: internal/tui/tui/focus.go, internal/tui/tui/app.go
---

## Problem

The retained TUI had ordered focus traversal for panes, but no way to return to
previously focused panes. Child widgets should continue receiving ordinary
arrow keys, while Alt+Left/Right should navigate pane visits.

## Cause

`FocusManager` tracked only the current index in the focus ring. `App.handleKey`
handled Tab traversal but had no history-aware navigation path.

## Resolution

`FocusManager` now records focus transitions and exposes `Back` and `Forward`,
skipping widgets absent from the current ring. `App.handleKey` dispatches
Alt+Left/Right before focused children and invalidates after a successful move;
the input parser already decoded modified CSI arrows. Regression tests cover
history traversal, forward-history truncation, runtime dispatch, and CSI input.

## Notes

Focus history is widget-identity based and survives focus-ring rebuilds, which
lets transient retained-tree overlays be skipped when they are no longer in the
active ring. A new focus visit after going back truncates forward history.
