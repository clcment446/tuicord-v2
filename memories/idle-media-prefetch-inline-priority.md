---
name: idle-media-prefetch-inline-priority
summary: Idle Shell ticks warm cached custom emoji and sticker media, while inline custom-media completion prioritizes favorites then the active guild.
tags: [#media, #cache, #prefetch, #emoji, #sticker, #picker, #idle]
impact: normal
commit: d718420 (dirty)
date: 2026-07-16
created_at: 2026-07-16T17:03:51+02:00
scope: internal/ui/media_prefetch.go, internal/ui/inline_picker.go, internal/ui/shell.go
---

## Problem

Custom emoji and sticker media was fetched only when rendered, and composer completion used fuzzy score alone across every guild.

## Cause

The picker catalog lacked source-guild and favorite metadata in its inline ordering path, and no idle task enumerated the catalog for the existing persistent media cache.

## Resolution

`idleMediaPrefetcher` starts on a Shell tick after user activity stops, fetches one catalog URL at a time through the cache-aware media fetcher, and cancels when keyboard, mouse, or paste activity resumes (`internal/ui/media_prefetch.go:12-100`, `internal/ui/shell.go:153-162`). URLs list the active guild first and skip Lottie stickers. Inline completion receives the persisted favorites and tiers entries as favorite, active guild, then other (`internal/ui/inline_picker.go:125-185`).

## Notes

The fetcher already follows HTTP cache policy and writes cacheable responses into the user cache, so prefetch uses the same cache rather than adding another store. `go test ./...` passed after this change.
