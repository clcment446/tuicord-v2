---
name: embed-component-border-presets
summary: Border presets are shared by embed/component frames and reusable widgets; configurable presets must preserve the existing curved rounded glyphs rather than widget.RoundedBorder.
tags: [#ui, #embed, #components, #border, #widgets, #config]
impact: normal
commit: 13b0f44 (dirty)
date: 2026-07-23
created_at: 2026-07-23T11:00:00+02:00
scope: internal/ui/embedview.go, internal/ui/componentview.go, internal/ui/border_style.go, internal/tui/widget
---

## Problem

Embed and component container frames were hardcoded in `frameEmbedLines` as
`╭╮╰╯─│`, so users could not select another border glyph set.

## Cause

Both renderers share the same frame helper, while `widget.RoundedBorder` is
named for the general widget default but contains square corners (`┌┐└┘`).
Using it as the chat-frame default would change existing output.

## Resolution

The UI now maps `display.border_style` presets (`rounded`, `square`, `heavy`,
`double`, and `ascii`) to explicit glyph sets. The selected set is passed into
`frameEmbedLines` and the reusable `Border`, `Split`, `Modal`, and `Menu`
widgets, covering both embeds and component containers. The rounded preset
explicitly retains the previous curved corners.

## Notes

Unknown values fall back to rounded. Targeted config and UI tests, plus the
full suite's non-network packages, pass.
