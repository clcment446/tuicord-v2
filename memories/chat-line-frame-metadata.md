---
name: chat-line-frame-metadata
summary: ChatView frame and prefix transforms must preserve and offset every chatLine metadata field, not only rendered segments and component hits.
tags: [#chat, #rendering, #embeds, #components, #markup, #interactions]
impact: high
commit: 368b257 (dirty)
date: 2026-07-17
created_at: 2026-07-17T00:00:00+02:00
scope: internal/ui/chat_line.go, internal/ui/embedview.go, internal/ui/componentview.go
---

## Problem

Framing embeds or component containers previously copied text and some media but dropped entity clicks, header collapse metadata, and spinner visibility.

## Cause

`chatLine` combines visual content with coordinate-bearing interaction and media state. The old ad-hoc transforms rebuilt only selected fields.

## Resolution

`translateChatLine` creates a shifted copy, offsets component/entity hit ranges and inline media, and preserves all other fields. Embed framing and component prefixes now use it. Regression tests cover framed metadata and visible header clicks.

## Notes

Any new `chatLine` transform should begin with `translateChatLine` before decorating segments; avoid mutating slices from cached source lines.
