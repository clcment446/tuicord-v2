---
name: startup-unread-ui-freeze
summary: READY froze the TUI because ningen.ChannelIsUnread could synchronously REST-fetch channel/guild/member/roles per channel, while bulk unread updates amplified into N+1 full UI callbacks.
tags: [#startup, #freeze, #read-state, #notifications, #performance, #rest, #tui]
impact: critical
commit: 5fde30e
created_at: 2026-07-22T20:15:00+01:00
scope: internal/app/app.go, internal/app/gateway_handlers.go, internal/accounts/manager.go, internal/ui/main.go
---

## Cause

`resetReadStateCache` ran inside the READY closure on the UI goroutine and called
`ningen.ChannelIsUnread` for every channel. In user-session/no-intent mode,
ningen's permission check falls back through Arikawa state methods that may make
roughly 4–5 synchronous REST calls per channel. A probe showed the old path
scaling from about 4 seconds for 10 entries to about 40 seconds for 100 entries.

`CHANNEL_UNREAD_UPDATE` also called `ReadState.MarkUnread` once per entry. Ningen
emits one asynchronous `read.UpdateEvent` per successful mark, while tuicord
posted its own bulk callback, producing N+1 UI refreshes. Each refresh rebuilt
channel, guild, and member UI. Initial `GUILD_CREATE` additionally fired both
full guild and generic refresh callbacks.

## Resolution

- Never call `ChannelIsUnread` from cache/UI paths. Derive status from in-memory
  read positions, store channel last-message watermarks, and local mute state.
- Apply `CHANNEL_UNREAD_UPDATE` as one cache batch and post one UI callback;
  do not mirror it through ningen `MarkUnread`.
- Include the derived guild cache in account unread status.
- Read-state refreshes rebuild guild/channel attention rows without rebuilding
  member/chat chrome.
- Avoid duplicate generic refresh after `GUILD_CREATE` and duplicate member
  refresh inside `MainView.Refresh`.

## Follow-up crash

The local mute helper added by this fix initially checked `App` and ningen state
but dereferenced `a.store` without a nil guard. A read update delivered to a
partially initialized App panicked in `Store.Channel`. `channelMutedLocal` now
treats a missing store as unmuted, and `app.New` establishes the stronger
invariant that its store is always non-nil (`259a3f8`). Regression tests cover
both the callback and constructor paths.

## Invariant

Notification badges must never perform REST or permission hydration. Bulk
Discord dispatches must remain bulk through cache update and UI invalidation.
Gateway callbacks must tolerate partial lifecycle state; constructors should
establish non-nil core dependencies where possible.
