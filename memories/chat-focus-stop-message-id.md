---
name: chat-focus-stop-message-id
summary: Latest-message focus used a removed chatFocusStop.message field instead of the stored message index.
tags: [#go, #chatview, #focus-navigation, #compiler-error]
impact: normal
commit: 9a26247
date: 2026-07-22
created_at: 2026-07-22T00:00:00+02:00
scope: internal/ui/chatview.go
---

## Problem

`go build ./cmd/tuicord` failed at `internal/ui/chatview.go:3110` because
`chatFocusStop` no longer had a `message` field.

## Cause

`chatFocusStop` stores the backing message position in its `msg uint32`
field. The `focusLatestStop` loop retained an outdated access to
`focusStops[i].message` after the focus-stop model changed.

## Resolution

The loop now resolves `focusStops[i].msg` through `w.msgAt` before computing
the placement key. A regression test covers focusing the latest message.

## Notes

The command build passes with a writable Go cache. The focused UI test remains
blocked by unrelated pre-existing undefined symbols in
`internal/ui/local_plugins_test.go`.
