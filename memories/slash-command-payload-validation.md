---
name: slash-command-payload-validation
summary: Slash-command catalog and TUI plumbing are implemented and cached, but Discord currently rejects the user-client type-4 payload with Unknown Integration; capture a real Desktop request before enabling invocation.
tags: [#discord, #slash-commands, #interactions, #autocomplete, #cache, #rate-limit]
impact: critical
commit: d718420 (dirty)
date: 2026-07-16
created_at: 2026-07-16T00:00:00+02:00
scope: internal/app/commands.go, internal/app/app.go, internal/ui/command_picker.go
---

## Completed

- Added cached contextual command discovery: guilds use
  `GET /guilds/{guild_id}/application-command-index`; DMs use the channel
  application-command search endpoint. Catalogs are immutable snapshots keyed
  by guild/channel and cached for five minutes; READY invalidates the cache.
- Added the experimental `/` command picker, gated by
  `integrations.slash_commands.enabled` (default `false`). The current UI only
  invokes no-option commands; typed command/option and autocomplete payload
  plumbing is implemented underneath it.
- Added type-2 command and type-4 autocomplete payload builders, nested option
  serialization, an explicit empty `data.attachments` array, and Discord-style
  decimal snowflake interaction nonces.
- Catalog commands retain their original JSON object in `ApplicationCommand`.
  The interaction payload echoes that raw object so fields unsupported by
  Arikawa (for example `contexts`, `integration_types`, and `dm_permission`)
  are not normalized away.
- Fixed the vendored Arikawa rate limiter to parse Discord's fractional
  `Retry-After` values (for example `1.226`) and added a regression test.
- `env GOCACHE=/tmp/awesomeProject-go-build go test ./... -count=1` passed.

## Live validation

User-authorized tests targeted only the `Heavenly Dao | 天道宗` guild and its
`Heavenly Dao` application:

- `/equip` type-4 autocomplete was retried after raw-command preservation,
  explicit attachments, and snowflake nonces. Discord still returned HTTP 400
  `Unknown Integration`.
- Type-2 `/tutorial` invocation was deliberately not retried after this result
  to avoid unnecessary external effects and further rate-limit pressure.
- A first attempt to fetch the catalog after the autocomplete request exposed
  the fractional `Retry-After` parser issue; that is now fixed locally.

## Remaining work

1. Capture a real Discord Desktop network request for Heavenly Dao `/equip`
   autocomplete (or a known-safe command) and compare every field/header with
   the payload produced here. The headful browser tool was unavailable in this
   SSH session because it lacked an X server; use an existing logged-in Desktop
   client/devtools capture instead of repeated blind API probes.
2. Update the payload model from that fixture, then rerun the guarded type-4
   test. Only after it succeeds, run the guarded no-argument type-2
   `/tutorial` test once.
3. Keep slash-command integration disabled by default until both live tests
   pass. Do not log `.env`'s `TOKEN`, raw authorization headers, or captured
   request bodies containing secrets.
4. After the command integration is genuinely validated, resume the deferred
   local session-daemon task. It should use a same-host Unix socket with strict
   permissions, keep the token and gateway session in the daemon, and expose
   a client IPC protocol rather than distributing the token.
