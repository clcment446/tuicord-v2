---
name: reply-forward-reference-rendering
summary: convertMessage must map Reference/ReferencedMessage/MessageSnapshots into store.Message.Reply and .Forwards; forwards render through a synthetic message keyed by a fwd nonce so media placements stay unique.
tags: [#discord, #reply, #forward, #convert, #rendering, #chat]
impact: high
commit: pending
date: 2026-07-21
created_at: 2026-07-21T00:00:00+01:00
scope: internal/app/convert.go, internal/ui/replyview.go, internal/store/store.go
---

## Problem

Replies showed no referenced-message context (#26) and forwarded messages
rendered empty (#27): `convertMessage` dropped `Reference`,
`ReferencedMessage`, and `MessageSnapshots` entirely.

## Resolution

- `store.Message` gains `Reply *MessageReply` and `Forwards []ForwardedMessage`.
- `convertReply` distinguishes replies from crossposts (both carry a Default
  reference) via `discord.InlinedReplyMessage`; a nil `ReferencedMessage` on a
  reply means the original was deleted (`Reply.Deleted`).
- `renderReplyLine` draws "╭─▸ @author preview" with the member's role color
  and a user-mention entity hit; `renderForwards` renders snapshot
  content/media/embeds through the normal renderers using a synthetic message
  with `ID:0, Nonce:"fwd:<id>:<nonce>:<i>"` so media placement keys cannot
  collide with the outer message.
- `handleMessageUpdate` keeps `Reply`/`Forwards` (only overwrites when the
  patch carries them) — same class of bug as the earlier ComponentTree
  omission in [[rich-v2-message-update-tree]].

## Notes

Snapshots carry no author identity (Discord omits it); don't invent one.
Tests: `internal/app/reply_forward_test.go`, `internal/ui/reply_forward_test.go`.
