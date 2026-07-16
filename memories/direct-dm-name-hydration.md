---
name: direct-dm-name-hydration
summary: One-to-one DM startup payloads can omit recipients and must not overwrite a previously hydrated recipient name.
tags: [#discord, #dm, #startup, #hydration, #cloudflare]
impact: high
commit: d718420 (dirty)
date: 2026-07-15
created_at: 2026-07-15T15:20:00+02:00
scope: internal/app/convert.go, internal/app/app.go, internal/discord/client_test.go
---

## Problem

Discord's startup private-channel payload can contain a one-to-one DM with no
`Name` and no `DMRecipients`. The sidebar then stored the `DM <snowflake>`
fallback. Hydrating again from READY caused duplicate channel-detail requests
and triggered a Cloudflare error during launch.

## Cause

`LoadGuilds` already hydrates sparse private channels through the channel-detail
endpoint. READY can arrive afterward with a sparse copy and overwrite the richer
stored channel.

## Resolution

`ingestPrivateChannel` preserves an existing non-empty DM name when the incoming
channel has no name or recipients (`internal/app/convert.go:698-706`). READY does
not perform another REST hydration pass. The app regression test covers the
overwrite case, and `TestSessionFetchesUserDMNamesFromDiscordAPI` validates a
real one-to-one DM using `.env` credentials.

## Notes

The live test passed against Discord on 2026-07-15. Focused `internal/app` and
`internal/discord` suites also passed with `GOCACHE=/tmp/tuicord-go-cache`.
