---
name: split-rebuild-preserves-root-sizing
summary: Split.rebuild must mutate its root layout node in place; wholesale reassignment wiped externally-set Basis/Grow, making the composer row jump to ~50% height on the first divider drag (issue #25).
tags: [#tui, #split, #layout, #resize, #composer, #accounts]
impact: normal
commit: 459ade4 (dirty)
date: 2026-07-21
created_at: 2026-07-21T00:00:00+02:00
scope: internal/tui/widget/split.go
---

## Problem

Dragging the account picker divider (or clicking it to collapse) made the
accounts|composer row grow to about half the chat column's height instead of
keeping its pinned composer height (GitHub issue #25).

## Cause

compose() pins the composer row by setting Basis/Grow directly on the split's
root layout node (`chatColumn.Children()[1].Layout()`, which is `&Split.node`).
Split.rebuild() — called on every Basis/drag/collapse/Hide mutation — replaced
the whole node with `w.node = layout.Node{Dir, Grow: 1, Gap, Children}`,
resetting Basis to 0 and Grow to 1. The row then competed with the chat pane
(also Grow 1) and split the column 50/50. The paste-preview growth, which also
mutates that node's Basis, was clobbered the same way.

## Resolution

rebuild() now mutates only Dir, Gap, and Children in place; the root node's
own sizing fields (Basis/Grow/Min/Max/Hidden) are never touched after
construction. The Grow: 1 default moved to NewSplit. Regression test:
TestSplitRebuildPreservesExternalRootSizing in split_test.go.

## Notes

Any widget that exposes its layout node via Layout() must treat that node as
shared state owned jointly with the composed tree: internal rebuilds may
replace children but must never reassign the node wholesale.
