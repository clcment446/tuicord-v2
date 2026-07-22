---
name: gateway-ingress-ordering-and-readmark-snapshot
summary: Gateway handlers must use AddSyncHandler (arikawa AddHandler runs each callback in its own goroutine, reordering create/delete), and read-state attention must read an App-owned readMark snapshot, never ningen's ReadState() pointer.
tags: [#gateway, #concurrency, #read-state, #ordering, #race]
impact: critical
commit: 487094a
created_at: 2026-07-22T20:30:00+01:00
scope: internal/app
---

## Root causes

- Arikawa `ophandler.Loop` reads socket events in order on ONE goroutine and
  calls `Handler.Call`, but `AddHandler` dispatches each callback via
  `go h.call(event)`. So handler bodies run concurrently and a CREATE dispatched
  before a DELETE can enqueue its `ui.Post` after the DELETE's -> resurrection.
  Fix: register all app gateway handlers with `AddSyncHandler` (runs inline,
  preserving dispatch order into the Post FIFO). Safe because `tui.App.Post` is a
  non-blocking mutex append; handler bodies only convert + Post.

- ningen `read.State.ReadState()` returns an internal `*gateway.ReadState` after
  releasing its mutex; the gateway mutates those fields concurrently. Reading it
  in `localReadState` (every sidebar render) and `MarkRead` raced under `-race`.
  Fix: `App.readMarks map[ChannelID]readMark` guarded by `unreadMu`, fed only by
  value copies — READY's `e.ReadStates` (+ `read_state.entries` raw-body fallback
  that ningen also parses; `ReadyEventKeepRaw` is enabled in ningen.go) and
  `read.UpdateEvent` (embeds a value copy). Pruned in `removeCachedReadState`,
  rebuilt via `replaceReadMarks` on READY.

## ningen's two handlers (non-obvious)

`ningen.State` embeds BOTH `*state.State` (whose session Handler is the
"prehandler") AND its own `*handler.Handler`. ningen's per-state handlers
(MutedState, ReadState, ...) register on the prehandler; a forwarding sync
handler on the prehandler calls `ningen.Handler.Call(v)`. `a.handle.AddSyncHandler`
resolves to the depth-1 embedded `ningen.Handler`, so OUR app handlers live there
and run (in order) when the prehandler forwards. In tests, dispatch state-level
events (READY, USER_GUILD_SETTINGS_UPDATE) via `ning.State.Handler.Call(ev)` to
reach ningen's prehandlers; `ning.Call(ev)` only hits ningen.Handler. Mute maps
are nil until a READY is dispatched.

## Gotchas

- ningen does NOT emit `read.UpdateEvent` for READY-seeded states, only for
  live acks/unreads — so READY seeding must parse the event, not wait for events.
- ningen emits `read.UpdateEvent` from `go func(){ state.Call(...) }`, so those
  are unordered among themselves regardless of sync handlers.
- `cacheReadState`/`cacheReadStateBatch` run in the handler goroutine (not Post),
  which is why `unreadMu` exists; readMarks share it. See [[regression-audit-ef79c17-head]].
