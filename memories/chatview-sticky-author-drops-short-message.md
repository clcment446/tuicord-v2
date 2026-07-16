---
name: chatview-sticky-author-drops-short-message
summary: Sticky author replacement can drop the final one-line message when a viewport starts inside a wrapped message.
tags: [#chat, #viewport, #rendering, #author, #wrapping]
impact: high
commit: 1915d44 (dirty)
date: 2026-07-15
created_at: 2026-07-15T13:40:03+02:00
scope: internal/ui/chatview.go:297-319, internal/ui/chatview_test.go, internal/tui/widget/bottom_scroll.go
---

## Problem

When a fat wrapped message is immediately above a short message, the short
message's only content row can disappear at the bottom of the chat viewport.

## Cause

`ChatView.Draw` computes a height-sized `displayLines` slice, then prepends a
sticky author row when the slice starts inside message content. The code at
`internal/ui/chatview.go:312-317` removes the last row to keep the slice at
viewport height, but that row may be the next message's only content line.

## Resolution

The issue was reproduced with a 20-column, 4-row viewport: the raw rendered
sequence contained the short message, while the drawn sequence replaced its
content with the fat message's author. The separate incoming-content scroll
compensation was moved into `internal/tui/widget.BottomScroll`, which now
preserves a nonzero bottom offset as content grows. The sticky-author rewrite
at `internal/ui/chatview.go:306-312` now replaces the oldest visible content
row rather than removing the newest row. Sticky-author replacement is skipped
when the viewport has only one row, where newest content takes priority. The
draw-after-Esc and one-row regressions in `internal/ui/chatview_test.go` assert
the pinned author and newest message remain visible at their respective sizes.

## Notes

`go test ./... -count=1` passes with the sticky-author fix and regression.
