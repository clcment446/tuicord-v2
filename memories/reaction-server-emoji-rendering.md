---
name: reaction-server-emoji-rendering
summary: Custom server emojis in reaction summaries use inline CDN images and no colon markers.
tags: [#reactions, #emoji, #kitty, #media, #discord]
impact: normal
commit: 558bd34 (dirty)
date: 2026-07-14
created_at: 2026-07-14T12:00:00+02:00
scope: internal/ui/richblocks.go, internal/ui/chatview.go
---

## Problem

Reaction summaries rendered custom server emojis as `:name:` text in
`internal/ui/richblocks.go:125-193`, even though message content already had
an inline custom-emoji media path.

## Cause

`reactionLabel` only returned text, and reaction rendering did not attach
inline media placements to the reaction line.

## Resolution

Custom reaction IDs now use the same 48px Discord CDN URL and Kitty inline
media pipeline as message-content emojis. The reaction line reserves a 2×1
cell slot, renders the count beside it, and falls back to the bare emoji name
without colon markers when media is disabled or unavailable. Placement IDs are
prefixed with the message identity to avoid collisions across messages.

## Notes

`internal/ui/richblocks_test.go:61-86` verifies both the absence of `:name:`
markers and the 2×1 graphics placement. `go test ./internal/ui` passes.
