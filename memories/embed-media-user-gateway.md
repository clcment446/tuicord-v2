---
name: embed-media-user-gateway
summary: Framed embed images must preserve cell borders and user sessions must bypass Discord's bot-only gateway REST lookup.
tags: [#embeds, #media, #kitty, #discord, #gateway, #auth]
impact: high
commit: f9fae04 (dirty)
date: 2026-07-14
created_at: 2026-07-14T00:00:00+02:00
scope: internal/ui/embedview.go, internal/ui/chatview.go, internal/discord/client.go
---

## Problem

Loaded images in non-pure link embeds were drawn at column zero, covering the
frame's left border and leaving image cells without the embed background.
Startup also received Discord HTTP 403 `Only bots are allowed to use this
endpoint` while opening a user-token session.

## Cause

`frameEmbedLines` passed media rows through without frame cells, and the Kitty
image widget clears its target cells before attaching terminal graphics.
Arikawa's gateway construction queried Discord's `/gateway` REST endpoint even
for a user token; that endpoint now rejects this request as bot-only.

## Resolution

Framed media rows now render border and fill cells first, offset the graphic by
one column, and pass the embed background as the image placeholder style
(`internal/ui/embedview.go:159-180`, `internal/ui/chatview.go:135-145,243-250,278-284`).
`NewSession` now creates the gateway from the public websocket URL while
retaining the configured browser-shaped REST client
(`internal/discord/client.go:52-61`). Regression coverage is in
`internal/ui/chatview_test.go:392-438`.

## Notes

Focused UI tests and Discord construction/header tests pass. Full Discord API
integration tests require network access and were unavailable in the sandbox.
