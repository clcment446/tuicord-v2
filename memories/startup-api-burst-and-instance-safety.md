---
name: startup-api-burst-and-instance-safety
summary: A user token got flagged (gateway close 4004) from a doubled startup REST burst; safe fixes are a single-instance flock, bounding DM hydration, and explicit 4004 messaging. Do NOT remove the pre-connect LoadGuilds — READY does not populate the guild list here.
tags: [#discord, #user-session, #startup, #rate-limit, #cloudflare, #gateway, #4004, #self-bot]
impact: critical
commit: 8c5aa42 (dirty)
date: 2026-07-20
created_at: 2026-07-20T00:00:00+01:00
scope: cmd/tuicord/main.go, cmd/tuicord/lock.go, internal/config/config.go, internal/app/app.go
---

## CRITICAL: LoadGuilds is load-bearing

The pre-connect `orch.LoadGuilds(100)` in `cmd/tuicord/main.go` is REQUIRED. An
attempt to remove it (on the theory that gateway READY is the authoritative
directory source) left the Servers panel completely empty — the user-session
READY does not reliably deliver the guild/DM list, so `handleReady`'s
`ingestGuild`/`ingestPrivateChannel` had nothing to ingest. The REST pull is the
directory source; keep it. Bounding its DM hydration is the correct burst fix,
not deleting the pull.

## Problem

An account was suspended/flagged with gateway close code 4004 ("authentication
failed") after the user DMed a bot with two tuicord instances open on one token.
4004 is a symptom: Discord invalidated the user token because the account was
flagged. This is inherent to the user-token (self-bot) model; the flag was
provoked by excess/un-genuine REST at launch, doubled by the second instance.

## Cause

1. No single-instance guard: two processes on one token = two gateway IDENTIFYs
   plus two independent startup REST bursts.
2. `cmd/tuicord/main.go` called `orch.LoadGuilds(100)` *before* connecting. That
   REST-pulled `GET /users/@me/guilds` + `PrivateChannels`, which the gateway
   READY payload already delivers (`handleReady` ingests `e.Guilds` and
   `e.PrivateChannels`). Redundant, and a genuine web client never hits
   `/users/@me/guilds` at launch.
3. `hydratePrivateChannels` fanned out one concurrent channel-detail request per
   sparse DM with no bound (the Cloudflare-error-at-launch pattern from
   [[direct-dm-name-hydration]]); ×2 instances made it worse.
4. Fatal 4004 surfaced only a generic "Gateway error" toast, so the user could
   not tell the token was invalidated.

## Resolution

- `acquireInstanceLock` (`cmd/tuicord/lock.go`) takes an exclusive
  `flock(LOCK_EX|LOCK_NB)` on `config.LockPath()` (beside config.toml); a second
  instance exits with `errAlreadyRunning`. Kernel-released, so no stale lock.
- `hydratePrivateChannels` now caps concurrency with a `dmHydrationConcurrency
  = 4` semaphore instead of firing one goroutine per sparse DM at once. This
  keeps the launch fetch under Discord's rate limits WITHOUT changing what
  loads. The pre-connect `LoadGuilds(100)` stays (see the critical note above).
- `main.go` inspects the gateway error; `ws.CloseEvent{Code: 4004}` shows an
  "Authentication failed — token invalid / account flagged, re-authenticate"
  message.

An earlier revision of this fix ALSO removed `LoadGuilds` and moved hydration to
a READY-path `hydrateReadyDMs`; that broke the guild list and was reverted. Do
not retry it without first proving READY actually carries the guilds.

## Notes

Verified `flock` rejects a second holder (EWOULDBLOCK) and re-acquires after
release. `go build ./...`, `go vet`, `go test -race ./internal/app
./internal/config ./internal/store`, and full `go test ./...` pass with
`GOCACHE=/tmp/tuicord-go-cache`. No hardening makes the user-token model
ToS-safe; these changes only cut accidental API abuse. Related:
[[direct-dm-name-hydration]], [[discord-gateway-store-lifecycle-correctness]],
[[user-session-channel-history]], [[login-auth-failures]].
