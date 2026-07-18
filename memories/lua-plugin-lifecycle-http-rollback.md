---
name: lua-plugin-lifecycle-http-rollback
summary: Plugin accessors use an atomic app snapshot, HTTP requests inherit the active Lua context, and startup registration uses owner stacks so rollback restores shadowed handlers and themes.
tags: [#plugins, #lua, #concurrency, #shutdown, #http, #rollback]
impact: critical
commit: pending
date: 2026-07-18
created_at: 2026-07-18T00:00:00+01:00
scope: cmd/tuicord/, internal/app/, internal/plugin/
---

## Shutdown-safe host reads

Never implement synchronous Lua accessors as an unconditional `App.Post` plus
channel receive. Plugins load before the UI loop starts, and `Manager.Close`
runs after the loop returns, so neither phase can drain the post. Publish the
UI-owned active guild/channel/self ID as an immutable `atomic.Pointer` snapshot
and let plugin accessors read it without touching an LState or waiting for UI.
Keep `Manager.Close` deferred after `Shell.Close` is deferred so LIFO ordering
cancels plugin work while Host targets remain alive.

## HTTP cancellation

`tuicord.http.get` runs synchronously on the single Lua worker. Build its request
with `http.NewRequestWithContext(L.Context(), ...)`; `safeDoFile` and `safeCall`
install the startup/callback deadline there, and runtime cancellation from
`Manager.Close` then interrupts transport and body reads. Do not use a separate
fixed client timeout or move Lua execution to another goroutine.

## Transactional registration

Commands, keys, and themes need per-name ownership stacks. The latest entry is
active, but shadowed entries remain available. If startup fails, remove all
entries owned by that LState before closing it: previous command/key/theme
owners become active again, newly introduced names disappear, and event
subscriptions owned by the failed state are filtered out. Theme entries must
carry explicit LState ownership even though palettes themselves contain no Lua
values.
