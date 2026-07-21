---
name: lua-config-theme-bootstrap-and-runtime-generation
summary: Lua config must execute transactionally before login/UI on the single plugin runtime; consume startup theme into typed Config before construction, then attach the live Host in place so bootstrap callbacks survive. Runtime themes must update App theme/background and bump shared style generation so chat bodies miss stale caches.
tags: [#lua, #config, #themes, #startup, #plugins, #uistate, #cache]
impact: critical
commit: dirty
date: 2026-07-20
created_at: 2026-07-20T00:00:00+01:00
scope: internal/config, internal/plugin, cmd/tuicord, internal/ui, internal/tui/tui
---

## Bootstrap ordering

Create one `plugin.Manager` with an inert Host and `ConfigTarget`, execute
`config.lua` once, consume its selected typed theme into `config.Config`, and
only then construct login/App/MainView/Shell. Populate the same Host object after
UI construction; replacing the manager or Host pointer loses config keymaps and
commands. Config mutation and selection are synchronous and never depend on
`App.Post`; live actions and runtime theme application Post after attachment.

## Transactions

Decode `tuicord.configure` into a working Config copy and commit only after the
whole Lua file succeeds. Theme/command/key ownership rollback remains per
LState. Defer startup `use_theme` side effects until plugin startup succeeds so
a later declaration error cannot apply a theme from a failed owner.

## Runtime repaint

A runtime theme must repopulate shared semantic Cells/overrides, call the safe
`tui.App.SetTheme` for background/toolkit state, refresh straightforward retained
MainView/Shell styles, and increment a shared style generation included in
`ChatView` body-cache validity. Updating Cells alone leaves App background and
cached message segments stale.

## Machine state

Account registry and preferred auth mode belong in the shared `uistate.State`,
not config persistence. MainView must receive that same State pointer; loading a
second copy lets later view-state saves overwrite newer account/auth churn.
