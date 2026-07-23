---
name: login-surface-background-holes
summary: The login screen punched terminal-default holes through the theme because unstyled widget.NewText labels and unstyled Split dividers clear their region with a background-less style. The tui app fills the theme background each frame, but those widgets overwrite it. Fixed by styling every login label (loginLabel helper, login.input cell), the Matrix status line (auth.status), and both login Split dividers (SetStyle panels.border). The Matrix panel added by feat/matrix roughly doubled the holey area.
tags: [#ui, #theme, #login, #matrix, #background, #consistency, #widget]
impact: medium
commit: 05a1e6e (dirty)
date: 2026-07-23
created_at: 2026-07-23T18:09:28+01:00
scope: internal/ui/login.go, internal/ui/login_matrix.go
---

## Root cause ("inconsistent theming" on the login screen)

`internal/tui/tui` fills the whole buffer with `Theme.Background` every frame
before drawing (app.go ~274), but widgets drawn on top can overwrite it:

- `widget.Text.Draw` calls `clear(r, w.style)` — an unstyled `NewText` fills its
  entire allocated region with a background-less `screen.Style{}`, erasing the
  themed backdrop. Text widgets also *grow* to fill leftover Column space, so a
  single label can blank many rows.
- `widget.Node` (Column/Row) `Draw` is a no-op and `widget.Border.Draw` only
  paints the frame — neither fills the interior, so an interior is themed only
  if the app fill survives or a child paints it.
- `widget.Split` draws its divider with `w.style`; the login splits called
  `SetBorderChars` but never `SetStyle`, so dividers drew background-less too.

Net: a render probe of the login tree (pre-filled with the theme bg) showed
~46% of cells with an UNSET background — visible as a patchwork on any terminal
whose default background differs from the theme. The `feat/matrix` branch added
a large, mostly-empty Matrix panel full of unstyled `NewText`, doubling it.
The QR panel was already correct (it fills its own background), so only the
Discord token panel + Matrix panel were holey.

## Fix

- `loginLabel(styles, content)` helper: `widget.NewText` + `SetStyle(
  styles.Cell("login.input"))` (fg text / bg background). All plain login
  labels in login.go and login_matrix.go go through it, including the empty
  `""` spacer rows (they blank rows too).
- Matrix `status` line: `SetStyle(styles.Cell("auth.status"))`.
- Both login Splits: `SetStyle(styles.Cell("panels.border"))` so the dividers
  match the themed frames.

Remaining unset cells are the text-input **cursor** positions
(`login.cursor` is intentionally background-less so the terminal draws its
cursor). Guarded by `TestLoginSurfaceFullyThemed` (renders buildLogin via
`tui.BuildHitIndex` + the same draw loop as `drawTree`, asserts <=6 unset-bg
cells).

## Related / still open

Same unstyled-`NewText` pattern exists in other overlays (help.go,
inline_picker.go, picker.go, forumview.go, forumpost.go) — not audited here;
most draw over already-themed main content so the holes are less visible.
The palette derivation itself is clean (main app resolves fully to the active
theme). Related: [[overlay-border-theming-seam]],
[[lua-config-theme-bootstrap-and-runtime-generation]].
