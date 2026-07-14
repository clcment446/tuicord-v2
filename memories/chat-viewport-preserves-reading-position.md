---
name: chat-viewport-preserves-reading-position
summary: Preserve a scrolled chat viewport when new message lines are appended below it.
tags: [#chat, #viewport, #scroll, #messages, #tui]
impact: normal
commit: 558bd34 (dirty)
date: 2026-07-14
created_at: 2026-07-14T12:15:00+02:00
scope: internal/ui/chatview.go, internal/ui/chatview_test.go
---

## Problem

When a user scrolled up to read older messages, a new message increased the
rendered line count and bottom-aligned rendering moved the viewport downward,
hiding the lines being read.

## Cause

`ChatView.Draw` calculated `start` from `len(lines) - height - scroll`, but
`scroll` represented distance from the bottom and was not adjusted when new
lines were appended.

## Resolution

`ChatView` now remembers the previous rendered line count and, only when
`scroll > 0`, increases the scroll offset by the number of newly rendered
lines (`internal/ui/chatview.go:287-301`). At the bottom, scroll remains zero
so new messages continue to appear immediately. The regression test at
`internal/ui/chatview_test.go:601-622` verifies the visible rows stay unchanged.

## Notes

The test uses the existing mouse-wheel scroll path and passes with
`go test ./internal/ui`.
