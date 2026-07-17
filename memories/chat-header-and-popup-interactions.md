---
name: chat-header-and-popup-interactions
summary: Chat header levels use per-level styles and collapse hit targets, while popup mouse events must preempt covered sidebar rows.
tags: [#chat, #headers, #collapse, #popup, #mouse, #kitty]
impact: normal
commit: ab0672e (dirty)
date: 2026-07-17
created_at: 2026-07-17T11:17:06+02:00
scope: internal/ui/chatview.go, internal/tui/tui/app.go, internal/ui/shell.go
---

## Problem

Message headers above level three were parsed as lower-level headings, and
there was no message-local collapse target. Popup menu clicks could also reach
the channel list underneath because popups are drawn as overlays but were not
part of the retained hit index.

## Cause

The Markdown scanner stopped counting `#` at three levels. The runtime routed
mouse events through hit-index children before the Shell's transient popup
handler.

## Resolution

Header parsing now accepts levels one through six, styles are selected by the
header level, and rendered headers expose a two-cell collapse marker. The TUI
runtime's `MouseOverlay` hook gives active Shell popups first refusal of mouse
events. Kitty media placements use a negative z-index so popup cell content is
not visually covered.

## Notes

`env GOCACHE=/tmp/tuicord-go-cache go test ./... -count=1` passed after the
change. Keep popup routing separate from normal retained hit testing so an
overlay can cover any underlying widget, not only channels.
