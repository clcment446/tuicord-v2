---
name: chat-prepend-scroll-teleport
summary: BottomScroll treats prepended older messages like appended newer content and shifts the viewport above its target.
tags: [#chat, #scroll, #viewport, #history, #prepend]
impact: high
commit: d718420 (dirty)
date: 2026-07-15
created_at: 2026-07-15T15:45:00+02:00
scope: internal/tui/widget/bottom_scroll.go, internal/ui/chatview.go, internal/app/app.go
---

## Problem

Scrolling upward near the history boundary can teleport the message viewport
farther upward than the user's one-row scroll target.

## Cause

`ChatView.scrollUp` increments the bottom-relative offset and triggers
`LoadOlderHistory`. `loadOlderHistory` prepends older messages via
`Store.PrependMessages`. `BottomScroll.Update` cannot distinguish prepend from
append, so when content grows while offset is nonzero it adds the entire line
delta to the offset (`internal/tui/widget/bottom_scroll.go:22-24`). For prepended
lines, that moves the computed start backward by the number of fetched lines,
showing much older content than intended.

## Resolution

`BottomScroll.UpdatePrepend` records prepended content without increasing the
bottom-relative offset. `ChatView.Draw` detects this case by comparing the
first message ID across frames and selects the prepend update path; ordinary
content growth continues using `Update` and its append compensation.

## Notes

`ChatView.Draw` computes the visible start as
`len(lines) - height - offset` (`internal/ui/chatview.go:345-350`). History pages
are added at the front (`internal/app/app.go:884-885`), confirming the direction
mismatch. Regression coverage is in `internal/ui/chatview_test.go` and
`internal/tui/widget/bottom_scroll_test.go`.
