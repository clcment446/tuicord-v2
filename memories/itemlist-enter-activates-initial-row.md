---
name: itemlist-enter-activates-initial-row
summary: ItemList needed explicit Enter activation because the initially selected first row does not emit a selection-change callback.
tags: [#itemlist, #keyboard, #selection, #tui]
impact: normal
commit: 558bd34 (dirty)
date: 2026-07-14
created_at: 2026-07-14T00:00:00+02:00
scope: internal/tui/widget/itemlist.go, internal/tui/widget/itemlist_test.go
---

## Problem

The first row in shared list controls was selected by default, but pressing
Enter did nothing. Users had to left-click the row to activate it because
keyboard handling only supported navigation (`internal/tui/widget/itemlist.go:228-267`).

## Cause

`ItemList.Handle` had no `input.KeyEnter` branch. Moving to a row uses
`SetSelected`, whose callback intentionally fires only when the index changes,
so the initial row could never submit from the keyboard.

## Resolution

Enter now invokes `onSelect` for the current row and is handled when the list is
non-empty; empty lists remain unhandled (`internal/tui/widget/itemlist.go:239-246`).
The initial-row behavior is covered by
`TestItemListEnterSelectsInitiallySelectedRow` (`internal/tui/widget/itemlist_test.go:88-101`).

## Notes

The focused widget/UI tests and full `go test ./...` suite pass.
