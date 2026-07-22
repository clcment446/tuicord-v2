---
name: explicit-chat-anchor-and-passive-latest
summary: Explicitly focused chat messages need a reusable top-row scroll anchor, while ESC returns navigation to a passive latest state.
tags: [#tui, #chat, #scroll, #focus, #embeds]
impact: high
commit: facd583 (dirty)
date: 2026-07-22
created_at: 2026-07-22T10:44:06+02:00
scope: internal/tui/widget/bottom_scroll.go:31-45, internal/ui/chatview.go:1161-1167, internal/ui/chatview.go:2451-2461, internal/ui/chatview.go:2986-3040
---

## Problem

When a message was explicitly focused by mouse hover, appended message rows
could move the viewport downward from the live edge. Components-v2 picker
collapse also needed to reuse the control-row pin. ESC left the old message
explicitly focused even after returning the viewport to latest.

## Cause

`BottomScroll.Update` only models bottom-relative append compensation. It has no
operation for a list whose explicit focus is a stronger top-row anchor. Chat
navigation also used `focusedExplicit` to distinguish passive latest state, but
ESC did not clear it and arrow navigation did not re-establish the latest stop.

## Resolution

Added `BottomScroll.UpdateAnchored`, used by ChatView only for explicit message
focus so appended rows preserve the previous logical top row. ESC clears
explicit focus; the first up/down action selects the newest focus stop. Picker
collapse records the same control anchor used by expansion.

## Notes

The reusable scroll primitive is covered by
`internal/tui/widget/bottom_scroll_test.go:39-50`. The full UI package could not
be compiled in this dirty worktree because unrelated `local_plugins_test.go`
references missing `newLocalCommandRegistry` and `localCommandPlugin` symbols.
