---
name: vim-composer-deferred-focus
summary: Vim i enables a composer that is absent from the current focus ring, so exact focus requests must survive until the next render rebuilds that ring.
tags: [#vim, #composer, #focus, #tui, #popup]
impact: high
date: 2026-07-18
scope: internal/tui/tui/app.go, internal/ui/shell.go, internal/ui/input_mode_test.go
---

## Root cause

In Vim normal mode `TextInput.CanFocus` is false, so the composer is omitted
from `FocusManager` during render. Pressing `i` made it focusable and requested
exact focus in the same event. `App.handleKey` immediately called `Focus.Set`,
but the current ring was stale and did not contain the composer; the failed
request was discarded. Direct Shell tests only checked `TakeFocusRequest`, so
they missed runtime focus behavior.

Transient context menus had the related routing problem: they draw above the
retained tree but did not implement `EventOverlay`, allowing the focused chat
to consume menu keys before Shell.

## Resolution

`App` retains an exact focus request that cannot yet be satisfied and retries it
after the next render's `Focus.Replace`. Focus requests are consumed after both
keyboard and mouse routing. Shell routes active popup events through
`HandleOverlay`, so menu selection preempts chat and its resulting composer
focus request follows the same deferred path.

Runtime-level tests render the actual Shell with a disabled composer, press
`i` or activate popup Edit, render again, and assert that focus belongs to the
newly enabled composer.
