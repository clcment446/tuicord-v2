---
name: sticky-author-media-row-collision
summary: A partially-scrolled media block anchors onto the pinned sticky-author row and its clear() erases the author name; clip media away from row 0 when a sticky author is pinned.
tags: [#chat, #rendering, #media, #scroll, #author, #sticky, #viewport]
impact: high
commit: 957698a (dirty)
date: 2026-07-20
created_at: 2026-07-20T18:08:26+0100
scope: internal/ui/chatview.go (Draw loop ~1059-1122), internal/tui/screen/region.go
---

## Problem

While scrolling, a message's user avatar stayed visible but the author name
vanished (blanked), leaving only the pfp. Reported as "pfp overlaps name; want
name on top".

## Cause

When the viewport begins inside a message, `ChatView.Draw` pins that message's
`authorLine` at row 0. If the same message's image had scrolled partly off the
top, the media block is drawn once at `y - line.mediaRow`, which reconstructs
the image's true top and lands on row 0 — the pinned header row. `Image.Draw`
begins with `clear(r, style)`, so the media cleared row 0's cells and erased the
author name (avatar at cols 0-1 survived because a separate kitty placement
re-covered it; the name is plain text at cols >=2 / col 0). Inline chat images
use z=-1 (below text), so this was a cell-clear collision, not a z-order issue.

## Resolution

`Draw` now tracks `stickyPinned`. When set, the media draw call clips its region
away from row 0 via a new `screen.Region.WithClip(rect)` that tightens the
visible clip while leaving `origin` unchanged — so image proportions and the
`y-mediaRow` anchor (which relies on `Bounds`) stay correct and only row 0 is
protected. Regression: `TestStickyAuthorSurvivesPartiallyScrolledMedia` in
`internal/ui/media_layout_test.go` (row 0 == "alice"; fails as "" without fix).

## Notes

Related: [[chatview-sticky-author-drops-short-message]],
[[focused-chat-frames-and-media]], [[chat-line-frame-metadata]]. Any future
overlay drawn after the pinned header must likewise respect row 0. `WithClip`
differs from `Clip`: `Clip` re-roots origin (changes local coords/proportions),
`WithClip` only narrows the clip.
