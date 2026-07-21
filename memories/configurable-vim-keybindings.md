---
name: configurable-vim-keybindings
summary: Issue #39 was fixed by moving modal Vim and panel-focus actions into typed keys.vim configuration with legacy defaults preserved.
tags: [#keybindings, #vim, #config, #tui]
impact: normal
commit: 05bf6e7 (dirty)
date: 2026-07-21
created_at: 2026-07-21T23:35:00+02:00
scope: internal/config/config.go, internal/tui/tui/app.go, internal/ui/chatview.go
---

## Problem

Issue #39 identified hardcoded Vim-mode and panel-focus keys. The literals were
in the chat, forum, shell, and TUI focus-routing paths.

## Cause

The existing `config.Keys` schema covered global actions but had no typed
representation for modal Vim actions or h/l/H/L focus traversal.

## Resolution

`config.VimKeys` now defines those actions and supplies the prior bindings as
defaults. Chat/forum views, composer input mode, and `tui.App` focus routing
consume the configured specs. `config_test.go` verifies nested key overrides.

## Notes

Use `keys.vim` in Lua/TOML-backed configuration to override or disable actions;
zero-value view/test constructors fall back to `config.Default().Keys.Vim`.
