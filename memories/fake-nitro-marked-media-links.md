---
name: fake-nitro-marked-media-links
summary: Fake-Nitro media uses marker-prefixed Markdown link labels so the markup renderer can select emoji or sticker media rendering.
tags: [#nitro, #emoji, #sticker, #markup, #picker, #media]
impact: normal
commit: d718420 (dirty)
date: 2026-07-16
created_at: 2026-07-16T16:47:41+02:00
scope: internal/picker/picker.go, internal/markup/parser.go, internal/ui/chatview.go
---

## Problem

Bare CDN URLs do not carry enough intent for the markup renderer to distinguish fake-Nitro emoji from stickers.

## Cause

The fallback picker functions returned only their CDN URLs, leaving the renderer to infer the desired presentation from a generic link.

## Resolution

`EmojiInsert` and `StickerInsert` emit `[emoji_<name>](<cdn-url>)` and `[sticker_<name>](<cdn-url>)` respectively (`internal/picker/picker.go:134-160`). `scanLink` converts those labels into dedicated spans while retaining the clean URL and name (`internal/markup/parser.go:238-260`). `ChatView` validates the CDN media class before rendering the emoji inline or the sticker as a block (`internal/ui/chatview.go:833-905`). When Discord supplies a matching minimal media embed, including a pretty-link `EmbedLink` whose original target is `Embed.URL` while its image is proxied, `renderEmbeds` skips that embed because the content span already renders it (`internal/ui/embedview.go:13-63`).

## Notes

Only marker-prefixed labels with a non-empty name are special; ordinary links remain `Kind_Link`. The UI rejects a marker link whose URL does not classify as the matching Discord media type, avoiding marker-triggered arbitrary fetches. Suppression compares every relevant embed URL field (`URL`, image, thumbnail, video) but requires an otherwise empty unfurl, so richer or unrelated embeds remain visible.
