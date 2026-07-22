---
name: read-state-cache-and-ui-decomposition
summary: Server dots must consume an event-driven per-channel/per-guild cache; the large App and ChatView implementations are split into focused files without moving Discord-specific behavior into the inner TUI package.
tags: [#performance, #read-state, #cache, #gateway, #tui, #refactor]
impact: high
commit: 3f57ede (dirty)
date: 2026-07-22
created_at: 2026-07-22T16:00:00+02:00
scope: internal/app/app.go, internal/app/gateway_handlers.go, internal/store/store.go, internal/ui/chatview.go
---

## Problem

Server badge rendering changed from the store's local ping aggregate to
`GuildUnread`, which scanned and sorted every channel during guild-rail
rebuilds. This made startup and read-state refreshes compete with chat startup.

## Cause

`GuildUnread` called `Store.Channels` for each guild and then invoked ningen's
channel unread checks. These are local state checks, not REST requests, but the
full-directory scan was still on the UI path.

## Resolution

`App` now caches channel states and guild aggregates under `unreadMu`.
`read.UpdateEvent` updates only the affected channel/guild, and accounts use a
dedicated guild-badge refresh callback. Local ping fallback aggregation is
constant-time through `Store.guildPings`; no badge render performs Discord REST.

`internal/app/app.go` was reduced to 642 lines, and `internal/ui/chatview.go`
to 754 lines. App responsibilities now live in `app_actions.go`,
`app_commands.go`, `app_directory.go`, `app_gateway_state.go`, and
`app_history.go`. ChatView responsibilities are split across lifecycle, media,
transcript, components, focus, and styles files. No extracted ChatView method
was moved into `internal/tui` because the candidate methods still depend on
ChatView/store/markup/media types rather than being reusable inner-TUI APIs.

## Notes

Focused race tests pass with `go test -race ./internal/app ./internal/accounts`.
The repository's full suite needed removal of orphaned untracked local-plugin
and autobot registry files; remaining network-listener tests require an
environment that permits `httptest` sockets.
