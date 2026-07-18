---
name: discord-gateway-store-lifecycle-correctness
summary: Initial history must merge in-flight gateway messages, prepends evict newest history, and partial MESSAGE_UPDATE payloads require JSON field-presence metadata.
tags: [#discord, #gateway, #history, #message-update, #store, #race]
impact: critical
commit: pending
date: 2026-07-18
created_at: 2026-07-18T20:24:26+01:00
scope: internal/app, internal/store, third_party/arikawa/gateway
---

## Problem

An initial REST history response replaced confirmed messages delivered by the gateway while the request was in flight. At full ring capacity, prepending older history immediately evicted that same older page. Arikawa represented MESSAGE_UPDATE as a full `discord.Message`, so omitted fields and explicitly empty/false fields were indistinguishable. Gateway lifecycle deletions also left cached state and active selections stale, and self-ID reads before posting could race READY on the UI goroutine.

## Resolution

`LoadHistory` snapshots cached message IDs on the UI goroutine and merges confirmed IDs first seen while REST is in flight. `PrependMessages` bounds from the newest end while reserving pending/failed local echoes. The local Arikawa `MessageUpdateEvent` unmarshaller records field presence, and app merging changes only present fields. Channel/guild lifecycle handlers now post mutations, cascade removals, preserve unavailable guilds, repair deleted active selections, and notify UI refresh callbacks. All self-ID-dependent gateway classification runs inside posted closures.

## Testing note

Async app tests must wait for the posted completion callback (`OnChange`/`OnReady`), not merely for the fake REST method to start or return; the latter can race the store mutation even with an immediate test poster.
