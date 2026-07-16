---
name: tui-focus-skips-split-separators
summary: Split dividers stay out of keyboard focus order by default, with accessibility.focus_splits enabling keyboard activation when needed.
tags: [#tui, #focus, #tab, #split, #separator, #collapsed, #accessibility]
impact: normal
commit: 498e27f (dirty)
date: 2026-07-15
created_at: 2026-07-15T00:00:00+02:00
scope: internal/tui/widget/split.go
---

## Problem

The retained focus walk included every Split because Split.CanFocus always
returned true. That made layout separators and collapsed section toggle strips
selectable with Tab/Shift+Tab, while users needing keyboard access had no way
to opt into split selectors.

## Cause

App.Render rebuilds its focus ring from every retained widget implementing
Focusable; Split is a layout/container widget and has no keyboard control
path, but advertised itself as focusable.

## Resolution

Split widgets implement a runtime focus policy. The default app setting keeps
CanFocus() false, while accessibility.focus_splits enables the selector in the
focus ring. When focused, Enter expands a collapsed split and Space
collapses/expands it; the divider is drawn bold as the focus indicator.
Mouse handling remains unchanged when accessibility.mouse_on is enabled.
Regression coverage in internal/tui/widget/split_test.go verifies default
exclusion, opt-in focus, forward/reverse Tab traversal, and both split sides.

## Notes

Keep layout affordances such as dividers out of the focus ring by default;
when opt-in focus is provided, give them an explicit keyboard interaction
contract and ensure parent focus indicators do not confuse descendant focus
with selector ownership.
