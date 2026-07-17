---
name: tty-16-color-mode
summary: The optional display.tty_colors setting quantizes emitted UI RGB styles to ANSI's standard 16-color SGR palette, using Kitty's active palette when available.
tags: [#tty, #colors, #ansi, #tui, #screen, #kitty]
impact: normal
commit: ab0672e (dirty)
date: 2026-07-17
created_at: 2026-07-17T00:00:00+02:00
scope: internal/config/config.go, internal/config/colors_conf.go, internal/ui/chatview.go, internal/tui/screen/diff.go, internal/tui/term/caps.go, internal/tui/tui/app.go
---

## Problem

The configured UI palette and dynamically generated role/embed/button colors were emitted as 24-bit ANSI sequences, which is unsuitable for terminals where the user wants the standard 16-color palette.

## Cause

`screen.Diff` and `screen.Frame` always encoded set colors with `38;2` and `48;2`; terminal capability probing did not select a color encoding mode.

## Resolution

`display.tty_colors = true` loads into `config.Display.TTYColors` and is passed to the TUI runtime via `WithTTYColors`. The screen frame emitter maps each set foreground/background color to the nearest standard ANSI 16-color entry and emits `30-37`, `90-97`, `40-47`, or `100-107` SGR codes. Existing `Diff` and `Frame` callers retain truecolor behavior by default.

## Notes

The quantization applies to cell-based UI styles, including runtime-generated colors. Kitty sessions load `color0`–`color15` from `~/.config/kitty/current-theme.conf` (the file managed by `kitten themes`) and fall back to the conventional palette. Raster terminal graphics remain raster graphics and are not converted by ANSI SGR quantization. Custom TOML colors require `[colors].enabled = true`; optional `colors.conf` rules are opt-in by file presence. The color override resolver is selector-driven and produces a complete `map[string]screen.Style` semantic cell palette, so guilds, channels, message markup, links, headers, embeds, components, menus, pickers, forms, auth, previews, and input cursor/placeholder cells can be overridden without adding parser code for each new property. Selectors support exact, one-segment wildcard, `{n}` header expansion, suffix wildcards such as `header*`, comma-separated property syntax, and independent fg/bg/attrs precedence. Full `go test ./...`, `go vet ./...`, and `git diff --check` pass with a workspace-local Go cache.
