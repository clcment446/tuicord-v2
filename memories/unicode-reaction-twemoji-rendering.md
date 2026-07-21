---
name: unicode-reaction-twemoji-rendering
summary: Standard Unicode reactions need Twemoji inline images because terminal fonts may not contain visible emoji glyphs; custom reactions already use Discord CDN images.
tags: [#reactions, #emoji, #twemoji, #kitty, #media]
impact: normal
date: 2026-07-21
created_at: 2026-07-21T18:20:07+01:00
scope: internal/ui/richblocks.go, internal/ui/chatview.go
---

## Problem

Custom Discord reaction emoji rendered correctly through Kitty graphics, but
standard Discord/Unicode reactions depended on the terminal font and could be
invisible.

## Cause

`renderReactions` only built image URLs when `Reaction.EmojiID != 0`. Unicode
reactions (`EmojiID == 0`) fell through to literal text.

## Resolution

Unicode reaction sequences are converted to pinned Twemoji 16.0.1 asset URLs
(code points joined with hyphens, with U+FE0F omitted) and use the same 2x1
inline-media path as custom reactions. Failed or temporarily unqueued media
falls back to literal emoji text instead of a blank slot. Saturated fetch queues
record a missing body dependency so cached fallback rows retry after capacity
returns.

## Verification

`internal/ui/richblocks_test.go` covers a rendered thumbs-up image and variation
selector normalization for the heart emoji. `go test ./...` passes.
