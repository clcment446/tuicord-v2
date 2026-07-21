---
name: named-group-dm-mention-recipients
summary: Named group DMs lost their recipients because hydration/preservation keyed off an empty name, emptying the @ mention menu.
tags: [#dm, #groupdm, #mentions, #autocomplete, #recipients, #hydration]
impact: high
commit: 61096ed (dirty)
date: 2026-07-21
created_at: 2026-07-21T00:00:00+01:00
scope: internal/app/app.go, internal/app/convert.go
---

## Problem

The `@` mention menu was empty in group DMs. The inline picker
(`internal/ui/inline_picker.go:107-115`) sources `@` members for a `ChannelDM`
from `channel.Recipients`, so an empty recipient list means no suggestions.

## Cause

Two places gated DM recipient handling on an empty *name* rather than on
missing recipients. Group DMs commonly carry a custom name, so both skipped them
when a sparse payload omitted recipients:

- `hydratePrivateChannels` (`internal/app/app.go`) skipped the detail fetch when
  `Name != "" || len(DMRecipients) > 0`, so a named group DM missing recipients
  was never hydrated.
- `ingestPrivateChannel` (`internal/app/convert.go`) preserved existing
  recipients only when `c.Name == ""`, so a later sparse READY/CHANNEL payload
  wiped a named group DM's already-hydrated members.

Unnamed group DMs (empty name) worked, which masked the bug as intermittent.

## Resolution

Both now key off missing recipients, not the name. Hydration fetches any
`DirectMessage`/`GroupDM` with `len(DMRecipients) == 0`. Ingestion preserves
existing `Recipients`/`RecipientIDs` whenever the new payload omits recipients,
and restores the name only when the raw `c.Name == ""` (note: `convertChannel`
fills a `"DM <id>"` placeholder, so check `c.Name`, not `converted.Name`).

Regression test: `TestReadyEventPreservesNamedGroupDMRecipients`
(`internal/app/app_test.go`). `GOCACHE=/tmp/tuicord-go-cache go test ./...`
passes.

See [[dm-mention-recipients-and-structured-hot-switch]] and
[[composer-inline-autocomplete]].
