---
name: itemlist-navigation-activation
summary: ItemList navigation must move focus without activating channel/server rows, and CSI Z is the standard Shift+Tab input.
tags: [#tui, #itemlist, #keyboard, #focus, #tab]
impact: normal
commit: d718420 (dirty)
date: 2026-07-15
created_at: 2026-07-15T15:35:00+02:00
scope: internal/tui/widget/itemlist.go, internal/tui/input/parser.go
---

## Problem

Arrow, page, and mouse-wheel movement in sidebar lists invoked `OnSelect`,
opening the newly highlighted channel/server immediately. Shift+Tab also did
not move focus backward because the parser discarded the terminal back-tab
sequence.

## Cause

`ItemList.Handle` used notifying selection for navigation. The input parser
handled CSI arrows and tilde keys but had no `CSI Z` case, which is the common
terminal encoding for Shift+Tab.

## Resolution

Navigation now uses silent selection; Enter and mouse clicks retain activation.
`parseLegacyCSI` maps final `Z` to `KeyTab` with the Shift modifier. Regression
tests cover ItemList keyboard/wheel navigation and parser decoding.

## Notes

`go test ./... -count=1` passed with `GOCACHE=/tmp/tuicord-go-cache`.
