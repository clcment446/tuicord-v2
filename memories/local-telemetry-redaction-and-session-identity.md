---
name: local-telemetry-redaction-and-session-identity
summary: Local Discord diagnostics capture REST and gateway failure context in bounded redacted session files; every row carries epoch-plus-token-hash sessionID without storing the raw token.
tags: [#telemetry, #discord, #privacy, #auth, #debugging]
impact: high
commit: a79197b (dirty)
date: 2026-07-28
created_at: 2026-07-28T00:00:00+02:00
scope: internal/telemetry, internal/discord/transport.go, cmd/tuicord/main.go
---

## Problem

Support investigations needed endpoint, request/response, permission, and
gateway-close context for early token pruning, but the token itself must never
be included in a developer export.

## Cause

REST requests shared the central `internal/discord` transport, while effective
permissions lived in `internal/store` and gateway close events were separate;
there was no local diagnostic session or export path correlating them.

## Resolution

`internal/telemetry` now writes bounded `0600` JSONL session files below the
application config directory and exports the latest session through
`tuicord --export-telemetry PATH`. The transport records sanitized endpoints,
headers, bodies, response status/headers/bodies, channel IDs, latency, and
permission snapshots. Session rows use `sessionID` formatted as Unix epoch plus
the full SHA-256 hash of the token; the raw token is transiently hashed and is
not retained. Gateway close codes, including 4004, are recorded separately.

## Notes

The app installs a permission resolver on the recorder so transport events can
report known/unknown channel permissions without making the transport depend on
the store package. Full `go test ./...`, `go vet ./...`, and `git diff --check`
passed after the implementation.
