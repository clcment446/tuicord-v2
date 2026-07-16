---
name: discord-command-catalog-endpoints
summary: User-session command discovery uses contextual catalog endpoints and must be cached per guild or DM channel.
tags: [#discord, #slash-commands, #interactions, #cache, #user-session]
impact: high
commit: d718420 (dirty)
date: 2026-07-16
created_at: 2026-07-16T00:00:00+02:00
scope: internal/app/commands.go:16-128
---

## Problem

Arikawa exposes application-owner command registration endpoints, but no
user-session command discovery API. Reusing those endpoints would require app
credentials and would not return the commands contextual to the active guild
or DM.

## Cause

Discord's user client uses different contextual endpoints for application
command catalogs. The shape is not represented by Arikawa's public command
client.

## Resolution

On 2026-07-16, authenticated read-only probes confirmed:

- Guilds: `GET /guilds/{guild_id}/application-command-index` returns
  `applications`, `application_commands`, and `version`.
- DMs: `GET /channels/{channel_id}/application-commands/search?type=1&query=&limit=25`
  returns `applications`, `application_commands`, and `cursor`.

`internal/app/commands.go:50-70` uses the matching endpoint for each context.
`LoadCommands` caches immutable command snapshots per `(guild, channel)` for
five minutes and serializes refreshes (`:89-128`) to avoid repeated API calls.

## Notes

The probes printed only HTTP status, response keys, and counts; they did not
log the account token, identifiers, command names, or response bodies. The
outgoing type-2 command payload has not been live-validated because executing
an arbitrary third-party command is an external side effect. Keep command
invocation experimental until a user-authorized test command supplies a
captured fixture.
