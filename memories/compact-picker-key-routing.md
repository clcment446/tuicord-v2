---
name: compact-picker-key-routing
summary: Inline autocomplete popups must receive keyboard input before the focused composer.
tags: [#picker, #autocomplete, #keyboard, #focus, #tui]
impact: normal
commit: ab0672e (dirty)
date: 2026-07-17
created_at: 2026-07-17T11:30:00+02:00
scope: internal/tui/tui/app.go, internal/ui/inline_picker.go
---

## Problem

In compact layouts, typing after `:` or `%` left the inline picker showing its
initial catalog order, with numeric/`00` entries before the requested letters.

## Cause

The inline picker is a transient Shell popup drawn outside the retained focus
tree. The composer remained focused and consumed rune events before Shell's
popup handler could update the picker query.

## Resolution

The retained TUI runtime now lets an `EventOverlay` preempt focused children for
keyboard and mouse events. Shell popups consequently receive query runes first,
while normal overlays remain handled through the focus tree.

## Notes

`env GOCACHE=/tmp/tuicord-go-cache go test ./... -count=1` passes. Keep
transient popups either in the retained tree or behind an explicit preemptive
event interface; drawing an overlay alone does not make it input-modal.
