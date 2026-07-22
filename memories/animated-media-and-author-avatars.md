---
name: animated-media-and-author-avatars
summary: ChatView must use FetchGIF and visible-only tick advancement; video posters are safe only through Discord's attachment proxy, while author headers prefer guild avatars.
tags: [#chat, #media, #gif, #video, #avatar, #rendering]
impact: normal
commit: 11afe37 (dirty)
date: 2026-07-17
created_at: 2026-07-17T22:46:13+02:00
scope: internal/ui/chatview.go, internal/ui/richblocks.go, internal/app/convert.go, internal/tui/tui/app.go
---

## Problem

`media.FetchGIF` and the GIF frame decoder existed, but `ChatView` called only
`Fetch`, retaining the first frame forever. Video attachments were text chips,
and author headers had no avatar data or media placement.

## Cause

The chat media state represented only one `image.Image`; its 500 ms runtime
tick was not suitable for GIF delays. A direct video URL is MP4 data, not a
safe image-decoder input. Message conversion discarded author and member avatar
URLs.

## Resolution

`ChatView.fetchMedia` uses `FetchGIF` for GIF URLs, retains at most 120 frames,
and advances only placements drawn in the current viewport. The runtime heartbeat is 50 ms.
Video poster fetching is restricted to `Attachment.ProxyURL`, with `format=jpeg`
added; direct videos remain accessible text chips. `Message.AuthorAvatarURL`
and `Member.AvatarURL` flow from Discord conversion, and `authorLine` chooses
the guild avatar before the global avatar.

## Notes

Regression coverage is in `internal/ui/richblocks_test.go`; package checks
passed for `./internal/ui ./internal/store ./internal/media ./internal/tui/tui`.
