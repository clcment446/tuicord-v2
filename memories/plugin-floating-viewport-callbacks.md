---
name: plugin-floating-viewport-callbacks
summary: Lua plugin panels must be hosted as Shell popups and marshal their action callbacks back onto the single Lua runtime.
tags: [#plugins, #lua, #tui, #viewport, #drag]
impact: high
commit: facd583 (dirty)
date: 2026-07-22
created_at: 2026-07-22T10:04:19+02:00
scope: internal/plugin/api.go, internal/ui/plugin_viewport.go, internal/ui/shell.go
---

## Problem

`tuicord.overlay` swaps Shell's retained root and therefore covers the entire chat UI. Popup widgets are drawn outside that retained tree, so their title-bar drags are not discovered by normal hit testing.

## Resolution

`tuicord.viewport(title, lines, actions)` validates declarative Lua actions, then routes each selected action through the manager's bounded single-Lua-worker queue. `pluginViewport` wraps `widget.Modal` and is installed as a non-modal Shell popup, preserving both the main UI and Vim INSERT focus. The runtime asks `OverlayHitTester` for the component beneath a transient pointer event, then starts the component's own drag or resize operation; Shell only supplies z-order.

## Notes

Never invoke a captured Lua action directly from the UI goroutine. The plugin runtime is not goroutine-safe. Keep a visible popup out of `Shell.Children`; adding it to the retained tree changes the layout and makes it cover or resize the main view. Transient components implement `OverlayHit`, `Draggable`, and optionally `Resizable`; do not add Shell-specific title-bar or resize geometry.
