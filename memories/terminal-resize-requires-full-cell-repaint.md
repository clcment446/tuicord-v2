---
name: terminal-resize-requires-full-cell-repaint
summary: Terminal cell diffs must repaint the complete viewport after dimensions change because the terminal may reflow the previous grid.
tags: [#tui, #terminal, #resize, #rendering, #wrapping]
impact: high
commit: dirty
date: 2026-07-17
created_at: 2026-07-17T00:00:00+01:00
scope: internal/tui/screen/diff.go, internal/tui/screen/diff_test.go
---

## Problem

After resizing the terminal, borders and wrapped text could be drawn at stale
positions even though the retained layout and chat body cache used the new
width.

## Cause

The cell diff compared the new frame with the old fixed-size buffer and skipped
equal coordinates. A real terminal may reflow its existing contents as soon as
its width changes, so the old buffer no longer represents the physical screen.

## Resolution

`emitDiff` discards the previous cell grid whenever frame dimensions differ and
repaints every cell in the new viewport. `Frame` still retains the old buffer
for graphic cleanup and payload reuse. A regression test verifies that even
equal overlapping cells are emitted after a resize.

## Notes

`GOCACHE=/tmp/tuicord-go-cache go test ./... -count=1` passes.
