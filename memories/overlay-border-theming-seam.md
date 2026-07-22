---
name: overlay-border-theming-seam
summary: Every overlay/popup panel drew an unthemed border because the free titled() helper set an empty style, while main-layout panels used MainView.titled() with panels.border/panels.focus. titled() now takes Styles so all framed surfaces match; the picker tab bar was also the one unthemed picker element.
tags: [#ui, #theme, #overlay, #picker, #border, #consistency]
impact: medium
commit: f1b0d1d
created_at: 2026-07-22T23:10:00+01:00
scope: internal/ui
---

## Root cause (the "two UI implementations" the user saw)

`internal/ui` had **two** `titled()` helpers:

- free `titled(title, child)` in `main.go` — set `SetStyle(screen.Style{})`
  (an empty border style). Used by ~12 overlay/popup surfaces: emoji picker,
  inline/command/local-command pickers, login, help, quick switcher, prompt,
  forum post, server settings, and Shell independent overlays.
- `MainView.titled(title, child)` — applied `panels.border` / `panels.focus`
  and tracked the border in `themedBorders` for live re-theming.

So the persistent main layout was themed but every transient overlay frame was
not — the visible "screens don't fully use the theme" inconsistency.

## Fix

`titled` now takes the caller's `Styles` and applies `panels.border` /
`panels.focus`; all call sites thread their in-scope `styles`. `NewHelpOverlay`
gained a `Styles` param (it had none). Overlays are recreated per open, so they
pick up the current theme at construction and don't need `themedBorders`.

Also: the emoji picker styled `queryText`/`hintText`/`list` but never
`tabText` — now `tabText.SetStyle(styles.Cell("picker"))`.

`media_viewer` was already correct (prefers `mediaviewer.background` /
`mediaviewer.hint`, hardcoded only as fallback) — left alone.

## Still hardcoded (deferred, lower value)

- `shell.go` forum-preview popup background `screen.RGB(28,31,38)` — custom-drawn,
  not a widget; riskier to touch blind.
- `embedview.go` / `componentview.go` accent `screen.RGB(88,101,242)` — Discord
  brand-blurple fallback when an embed carries no color; defensible default.

Related: [[audit-fixes-devclaude-batch]],
[[lua-config-theme-bootstrap-and-runtime-generation]].
