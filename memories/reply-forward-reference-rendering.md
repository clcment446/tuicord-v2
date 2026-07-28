---
name: reply-forward-reference-rendering
summary: convertMessage maps reply/forward snapshots, but an ephemeral reply with an omitted ReferencedMessage is unavailable—not deleted—and must render a distinct notice.
tags: [#discord, #reply, #forward, #convert, #rendering, #chat]
impact: high
commit: 0264283 (dirty)
date: 2026-07-27
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
  reference) via `discord.InlinedReplyMessage`. A nil `ReferencedMessage` on a
  non-ephemeral reply means the original was deleted (`Reply.Deleted`), but an
  ephemeral reply can omit the snapshot while its original still exists. Mark
  that case `Reply.Unavailable` and render “original message is unavailable”,
  never `@unknown` or a false deletion notice.
- `renderReplyLine` draws "╭─▸ @author preview" with the member's role color
  and a user-mention entity hit; `renderForwards` renders snapshot
  content/media/embeds through the normal renderers using a synthetic message
  with `ID:0, Nonce:"fwd:<id>:<nonce>:<i>"` so media placement keys cannot
  collide with the outer message.
- `handleMessageUpdate` keeps `Reply`/`Forwards` (only overwrites when the
  patch carries them) — same class of bug as the earlier ComponentTree
  omission in [[rich-v2-message-update-tree]]. A sparse update can carry the
  reply reference without `ReferencedMessage`; its synthetic `Deleted` marker
  must not replace an already valid cached preview.
- Reply preview content is stored as raw Discord markup. Before collapsing it
  to one line, `renderReplyLine` must call `ChatView.displayContent`; otherwise
  mentions in the referenced message leak through as literal `<@user-id>`.

## Notes

Snapshots carry no author identity (Discord omits it); don't invent one.
Tests: `internal/app/reply_forward_test.go`, `internal/ui/reply_forward_test.go`.
