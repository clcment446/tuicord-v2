---
name: user-session-channel-history
summary: User-session channel views must avoid the bot-only active-thread REST endpoint and paginate older messages from the chat viewport.
tags: [#discord, #user-session, #history, #scroll, #chat]
impact: high
commit: 558bd34 (dirty)
date: 2026-07-14
created_at: 2026-07-14T00:00:00+02:00
scope: internal/ui/main.go, internal/app/app.go, internal/store/store.go, internal/ui/chatview.go
---

## Problem

Channel and forum selection invoked `LoadActiveThreads`, which requests
`GET /guilds/{guild}/threads/active`; user sessions can receive Discord's
`Only bots are allowed to use this endpoint` response. Chat history loaded one
latest page and scrolling only changed a local offset, so older messages were
never requested. A long message could also scroll its author line out of view.

## Cause

`internal/ui/main.go` called the active-thread REST fallback during guild,
channel, and forum navigation. `internal/app/app.go` had no
`MessagesBefore` path, and `internal/ui/chatview.go` had no top-reached loader
or pinned-author rendering.

## Resolution

User-session navigation no longer invokes the active-thread REST fallback;
gateway events remain the source for active thread state. `App.LoadOlderHistory`
uses Arikawa `MessagesBefore`, `Store.PrependMessages` adds older messages in
oldest-first order with ID deduplication, and `ChatView.OnReachTop` triggers the
next page. When a viewport starts inside a message, the sender line is pinned
under the channel title.

## Notes

The default suite and focused race tests pass. History is bounded by the
store's configured ring capacity, and pagination stops after a short page or
an empty result.
