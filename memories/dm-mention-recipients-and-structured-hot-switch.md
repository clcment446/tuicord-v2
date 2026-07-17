---
name: dm-mention-recipients-and-structured-hot-switch
summary: DM mention autocomplete needs channel-scoped recipients, while the plus switcher supports opt-in server and channel fuzzy syntax.
tags: [#dm, #mentions, #autocomplete, #search, #fuzzy, #channels]
impact: high
commit: dirty
date: 2026-07-17
created_at: 2026-07-17T00:00:00+01:00
scope: internal/store/store.go, internal/app/convert.go, internal/ui/inline_picker.go, internal/ui/main.go, internal/ui/shell.go
---

## Problem

Typing `@` in a DM produced no user suggestions because the synthetic Direct
Messages guild has no guild-member catalog. The `+` switcher only searched a
flat guild/channel label and could not explicitly scope a query to a server.
After selecting a recipient, the sent mention still rendered as
`@unknown-user` for the same reason: the markup resolver only consulted guild
members.

## Cause

Normalized DM channels discarded Discord's `DMRecipients`, and autocomplete
only queried `Store.Members(activeGuild)`. Switcher entries also had no separate
server and channel search keys.

## Resolution

DM channels now retain recipients as normalized members, including across
sparse READY payloads, and `@` uses recipients from the active DM channel.
Markup resolution and profile clicks first consult the active guild member
catalog, then fall back to the active DM channel's recipients.
The `+` switcher retains ordinary full-label fuzzy search and adds opt-in
`\server` and `\server#channel` fuzzy matching with separate keys. DM switcher
rows use `@name` labels.

## Notes

The test suite covers DM conversion and sparse-payload preservation, active-DM
mentions, structured server/channel matching, and unchanged ordinary search.
`GOCACHE=/tmp/tuicord-go-cache go test ./... -count=1` passes.
