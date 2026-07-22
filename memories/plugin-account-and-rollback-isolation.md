---
name: plugin-account-and-rollback-isolation
summary: Plugin Host actions resolve the active account via an accessor (not a captured launch App); events follow the active account via accounts.Manager sink binding; timer/viewport callbacks that survive startup rollback must check Manager.isLive before touching a possibly-closed LState.
tags: [#plugin, #multi-account, #concurrency, #rollback, #lua]
impact: critical
commit: 3a586be
created_at: 2026-07-22T21:15:00+01:00
scope: internal/plugin, internal/accounts, cmd/tuicord
---

## Account isolation (finding #4)

- `newPluginHost` takes `active func() *app.App` and resolves the account at call
  time for every action/accessor, so plugins act through the selected account.
- `accounts.Manager.Options.EventSink` binds the sink to the active account on
  each switch (detach others). Safe because `App.emit` runs only inside Post
  closures on the UI goroutine, and switch runs there too — `App.SetEventSink`
  is a plain field write, no lock. See [[gateway-ingress-ordering-and-readmark-snapshot]].

## Rollback / host concurrency (finding #5)

- Runtime is one worker goroutine (`internal/plugin/runtime.go`); load via `do`,
  callbacks via `submit`, both in submission order.
- On startup failure loadOne calls `rollbackRegistrations(L)` then `L.Close()`.
  Registry callbacks (events/commands/keys/themes) are cleaned by rollback, but:
  - timer ticks queued during the busy startup, and
  - viewport `on_press` closures the UI holds outside the registries,
  still reference L. Calling into a closed LState is a **Go nil-panic** —
  `safeCall`'s `Protect:true` only catches Lua errors — crashing the worker.
- Fix: both consult `Manager.isLive(L)` (membership in committed `m.states`)
  before `safeCall`; a rolled-back state is never committed.
- `AttachHost` copies `*host` via `rt.do` so it can't race a callback reading
  `m.host` on the worker.
