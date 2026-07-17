---
name: lua-plugin-system-concurrency
summary: The Lua plugin system runs all gopher-lua on one dedicated goroutine; effects marshal to the UI goroutine via App.Post, and every Discord snowflake ID crosses the Lua boundary as a decimal string.
tags: [#plugins, #lua, #concurrency, #app, #snowflakes]
impact: high
commit: 11afe37 (dirty)
date: 2026-07-18
created_at: 2026-07-18T00:00:00+02:00
scope: internal/plugin/, internal/app/app.go, internal/ui/shell.go, cmd/tuicord/plugins.go
---

## Design

`internal/plugin` embeds gopher-lua (Lua 5.1, pure Go). A gopher-lua `*LState`
is not goroutine-safe, so **all Lua executes on a single dedicated goroutine**
(`runtime.go`, bounded job queue). Inbound events enqueue non-blocking (dropped
if the queue is full, so a stuck plugin can't back-pressure the gateway).
Outbound side effects never touch the store/widgets directly — each binding
calls a `Host` func that marshals the real mutation onto the UI goroutine via
`App.Post`. Rule: **Lua runs on the plugin goroutine, effects land on the UI
goroutine.**

## Gotchas

- **Snowflakes as strings.** Lua numbers are float64 and lose precision above
  2^53, so every ID crosses the boundary as a decimal string (`convert.go`
  `toLua` renders `uint64` as string; `parseID` reads it back). App event
  payloads pass IDs as `uint64` and let `toLua` stringify.
- **Race-free accessors.** `tuicord.active_channel/guild/self_id` read App
  fields owned by the UI goroutine. The `cmd/tuicord/plugins.go` Host does a
  synchronous `Post`+channel round-trip to read them, never a direct field read
  from the plugin goroutine.
- **Decoupling seam.** `App.SetEventSink(EventSink)` + `a.emit(...)` in the
  gateway handlers keeps `internal/app` free of any import of `internal/plugin`;
  the `Host` is a struct of funcs (not an interface) wired in `main.go`.
- **Config comparability.** `Config.Plugins` is a `*Plugins` pointer because its
  `Disabled` slice / `Grants` map would otherwise break the `cfg != Default()`
  comparisons used across config tests. Nil pointer = enabled, none disabled.

## Runtime theming & custom UI

- `tuicord.style(selector, {fg=,bg=,attrs=,bold=,...})` mutates the shared
  `*config.ColorOverrides` (via new exported `SetProperty`) and calls
  `uiApp.Invalidate()`. It works live because `Styles.Cell(name)` resolves
  overrides at draw time through `Overrides.Resolve` and every widget shares the
  same `Overrides` pointer — no style rebuild needed. main.go allocates a
  non-nil `ColorOverrides` before building Styles so plugins have somewhere to
  write. Best-effort/additive: startup-baked `Cells` still apply, runtime rule
  overlays per-field.
- Custom UI is a **content-model overlay** (`tuicord.overlay(title, lines)`),
  not immediate-mode Lua drawing — Draw runs on the UI goroutine and cannot call
  the plugin-goroutine LState. `Shell.OpenPluginOverlay` renders plugin-supplied
  text lines; Esc dismisses via the existing overlay handling.

## Status

All four approved surfaces are built, wired, and tested (race-clean): events,
actions, `;`-commands, keybindings, runtime theming, and custom overlays.
Branch: devstyly. Plan: ~/.claude/plans/plan-the-lua-based-glowing-brooks.md.
