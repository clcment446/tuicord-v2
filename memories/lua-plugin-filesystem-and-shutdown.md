---
name: lua-plugin-filesystem-and-shutdown
summary: Plugin filesystem grants use per-plugin os.Root handles, and runtime cancellation interrupts deadline-bound Lua startup/callback execution before states are closed.
tags: [#plugins, #lua, #concurrency, #filesystem, #security]
impact: critical
commit: dirty
date: 2026-07-18
created_at: 2026-07-18T20:45:13+01:00
scope: internal/plugin/
---

## Confinement

An `fs` grant is exposed only when `Options.DataDir` is non-empty. The manager
creates and opens each plugin child directory through an `os.Root` rooted at
DataDir, then all read/write/list operations use that child Root. Do not replace
this with lexical `filepath.Join` checks: those do not safely handle symlinks or
path-swap races. `os.Root` rejects absolute names, `..` escapes, and symlinks
that resolve outside the child root.

## Lua shutdown

Every `DoFile` and callback installs a deadline context whose parent is owned by
the single-goroutine runtime. `Close` cancels that parent before waiting for the
worker, interrupting infinite Lua loops without calling `L.Close` concurrently.
Only after the worker exits does Manager close LStates and filesystem Roots.
Failed startup registrations must be removed before closing the failed state.
Logging has its own mutex because queue-drop logs can come from concurrent Emit
callers.
