---
name: forum-dm-media-loading
summary: Forum archive responses need parent guild context, sparse user-session DMs need channel hydration, and media must load asynchronously with response cache policy.
tags: [#forums, #archived-threads, #dm, #media, #cache]
impact: high
commit: 558bd34 (dirty)
date: 2026-07-14
created_at: 2026-07-14T11:38:31+02:00
scope: internal/app/threads.go, internal/app/app.go, internal/media/fetch.go, internal/ui/chatview.go
---

## Problem

Archived forum posts returned by Discord may omit `guild_id`, and user-session startup directory responses may omit DM recipients. Media was already fetched on worker goroutines, but loading state had no animated indicator and response cache directives were ignored.

## Cause

Archived threads were converted without inheriting the parent forum's guild. DM labels depended only on the sparse directory payload. The media cache wrote every successful response regardless of `Cache-Control`, and the UI used a static loading label.

## Resolution

Archived thread conversion inherits the parent forum guild (`internal/app/threads.go:86-92`). Missing DM recipient data is hydrated concurrently through the channel endpoint before startup channels are cached (`internal/app/app.go:1035-1059`). Media fetch/decode remains off the UI goroutine, uses eight concurrent slots, honors `no-store`/`no-cache`/`max-age=0`, and displays a braille spinner while loading (`internal/media/fetch.go`, `internal/ui/chatview.go:125-181`).

## Notes

The full suite's Discord package tests require live DNS/network access in this environment; focused app, UI, and media suites pass.
