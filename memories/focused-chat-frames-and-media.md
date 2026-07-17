---
name: focused-chat-frames-and-media
summary: Focus styling must apply to inline-media placeholder cells while excluding framed embed borders; embed headings inherit messages.header{n} colors.conf rules through markup rendering.
tags: [#chat, #focus, #embeds, #media, #colors, #vim]
impact: normal
commit: 368b257 (dirty)
date: 2026-07-17
created_at: 2026-07-17T16:20:00+02:00
scope: internal/ui/chatview.go, internal/ui/embedview.go, internal/config/colors_conf.go
---

## Problem

Focused chat blocks colored the cell text but `drawInlineMedia` subsequently
cleared image placeholder cells with their unfocused style. Whole-block
highlighting also colored the box-drawing cells around embeds.

## Cause

Inline media is rendered after its containing `chatLine`, so it must receive
the same focused style explicitly. Embed frames did not retain any metadata
that distinguished border cells from interior content cells.

## Resolution

`drawInlineMedia` now accepts focus state and applies `messages.focused`; the
default focused cell uses reverse video and remains configurable through
`messages.focused.fg` and `.bg`. `frameEmbedLines` marks its content bounds,
and focused drawing clips highlights to those bounds. Embed titles still flow
through `renderContent`, so header spans use `messages.header{n}` overrides.

## Notes

Regression coverage is in `internal/ui/embedview_test.go` and
`internal/ui/chatview_test.go`. Keep `messages.focused` semantic rather than
hard-coding a blue color so colors.conf controls it consistently.
