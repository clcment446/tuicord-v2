---
name: sticky-message-viewport-anchoring
summary: A scrolled chat viewport re-anchors to the message at its top by identity each draw; BottomScroll's append-below growth assumption teleports the view when content above changes height.
tags: [#chat, #scroll, #viewport, #anchoring, #components-v2, #fold, #config]
impact: high
commit: pending
date: 2026-07-21
created_at: 2026-07-21T00:00:00+01:00
scope: internal/ui/chat_anchor.go, internal/ui/chatview.go (Draw), internal/config/config.go
---

## Problem

Folding/unfolding embed v2 lists, async media loads, and edits above a scrolled
viewport shifted the reading position (issues #28, #29). `BottomScroll.Update`
treats all content growth as appended below the reading position
(`offset += delta`), which is wrong when lines change above the viewport top.

## Resolution

`ChatView.Draw` captures an anchor after computing `start`: the placement
prefix of the top visible line's message, its line index within that message's
block (`anchorIntra`), the screen-row delta to the first keyed line
(`anchorDelta`), and the offset it was captured under (`anchorOffset`). The
next draw re-derives the offset from the anchor message's new position —
overriding BottomScroll's guess — but only when the pre-update offset still
equals `anchorOffset` (otherwise the user scrolled and the stale anchor must
not fight them), the offset is > 0 (bottom-anchored views follow new
messages), and the channel is unchanged. `display.sticky_anchor` (default
true, issue #30) disables it via `SetStickyAnchor`.

## Notes

- `applyVimBoundaryFocus` runs after the restore, so gg-stick still wins.
- Anchor restore reproduces `UpdatePrepend` semantics for history prepends, so
  both paths agree.
- Tests: `internal/ui/chat_anchor_test.go`; remember sticky-author pinning
  replaces row 0 with the author line, so assertions check rows 1+.
- Related: [[chat-prepend-scroll-teleport]],
  [[chat-viewport-preserves-reading-position]],
  [[sticky-author-media-row-collision]].
