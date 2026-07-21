---
name: split-responsive-hide-pane-wrapper
summary: Responsive members hiding must apply to Split's generated pane wrapper, not the nested members widget, or the invisible pane keeps its minimum width and clips chat to zero on narrow terminals.
tags: [#tui, #layout, #split, #resize, #members, #responsive]
impact: high
commit: dirty
date: 2026-07-21
created_at: 2026-07-21T00:00:00+01:00
scope: internal/tui/widget/split.go, internal/ui/main.go, internal/tui/widget/split_test.go
---

## Problem

Below `layout.members_hide_below`, the members contents disappeared but its
20-column split pane and divider remained in layout. The chat therefore kept
shrinking as the terminal narrowed and reached zero width around 46 columns,
which looked like rendering had stopped.

## Cause

`MainView.compose` assigned `HideBelow` to `members.Layout()`. `Split.rebuild`
wraps each child in a generated pane node carrying `MinSecond`; the layout
solver only filters immediate children. Hiding the nested child did not hide
its wrapper or reclaim its minimum width/gap. `MembersAutoHide` was also never
read.

## Resolution

`Split.HideSecondBelow` stores the responsive policy on the Split and reapplies
it to every generated second-pane wrapper during rebuilds. MainView invokes it
only when `MembersAutoHide` is true. Below the threshold the solver removes the
whole pane, including its minimum width and divider gap, so chat reclaims the
available width. Regression coverage verifies threshold behavior, reclaimed
width, and policy survival after a Basis rebuild.
