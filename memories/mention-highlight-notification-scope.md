---
name: mention-highlight-notification-scope
summary: Persist self-ping classification on messages and derive role-colored chat highlights without reading the UI-owned store off-goroutine.
tags: [#mentions, #notifications, #unread, #chat, #tui, #race]
impact: normal
commit: 951f159 (dirty)
date: 2026-07-28
created_at: 2026-07-28T09:24:00+02:00
scope: internal/app/app_gateway_state.go, internal/app/app_history.go, internal/ui/chatview_transcript.go
---

## Problem

Mention highlighting must apply to both gateway messages and REST-loaded history, while channel badges need to distinguish mention counts from ordinary unread counts. The store is UI-goroutine owned.

## Cause

Live notification classification already depended on structured Discord mention fields, but that result was not retained on the normalized message. Classifying history messages in the REST goroutine would read members, roles, and channels concurrently with UI mutations.

## Resolution

`store.Message.PingsSelf` now carries the classification. Gateway and history paths assign it inside their posted UI closures. Chat rendering applies the configured mention style and the logged-in member's effective role color; sidebar channel badges show pings first and unread counts otherwise. Unknown guilds do not generate local notification state.

## Notes

Keep notification classification on the UI goroutine. The focused package race suite passed with `go test -race ./internal/app ./internal/ui ./internal/store -count=1`.
