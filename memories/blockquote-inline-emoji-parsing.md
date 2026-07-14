---
name: blockquote-inline-emoji-parsing
summary: Blockquote parsing must preserve inline custom emoji metadata for the UI media renderer.
tags: [#blockquote, #emoji, #markup, #rendering]
impact: normal
commit: 558bd34 (dirty)
date: 2026-07-14
created_at: 2026-07-14T13:58:09+02:00
scope: internal/markup/parser.go, internal/markup/markup.go, internal/ui/chatview.go
---

## Problem

Custom emoji in Discord blockquotes were displayed as literal ` <:name:id>`
syntax instead of rendered emoji. `internal/markup/parser.go:108-123` emitted
the complete quoted body as one `Kind_Quote` span, so the UI never received a
`Kind_Emoji` span.

## Cause

`scanQuote` intentionally skipped all inner inline parsing. This affected both
single-line `> ` quotes and message-wide `>>> ` quotes.

## Resolution

`emitQuotedLines` now emits the visual gutter separately and parses each quoted
line with the normal parser (`internal/markup/parser.go:126-140`). Parsed spans
carry `Span.Quoted` (`internal/markup/markup.go:132-138`), allowing
`internal/ui/chatview.go:642-646` to retain muted quote styling while the
existing custom-emoji media path handles the emoji.

## Notes

Configured muted backgrounds could overwrite embed/component backgrounds on
quoted emoji. `markupStyle` now restores the incoming background after applying
muted foreground/attributes. Focused `go test ./internal/markup ./internal/ui`
passes.
