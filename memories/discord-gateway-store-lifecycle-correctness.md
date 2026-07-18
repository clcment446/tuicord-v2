---
name: discord-gateway-store-lifecycle-correctness
summary: Async store loaders require generation and gate-version checks; thread sync is authoritative; bounded tombstones need an overflow watermark for in-flight history.
tags: [#discord, #gateway, #history, #message-update, #store, #race]
impact: critical
commit: pending
date: 2026-07-18
created_at: 2026-07-18T20:24:26+01:00
scope: internal/app, internal/store, third_party/arikawa/gateway
---

## Problem

An initial REST history response replaced confirmed messages delivered by the gateway while the request was in flight. A first merge fix still overwrote in-flight edits, resurrected deletes, and could recreate a message ring after channel deletion. At full ring capacity, a prepend strategy that retained the fetched page dropped gateway arrivals made during the request. Arikawa represented MESSAGE_UPDATE as a full `discord.Message`, so omitted fields and explicitly empty/false fields were indistinguishable. Gateway lifecycle deletions also left cached state and active selections stale, and self-ID reads before posting could race READY on the UI goroutine.

## Resolution

History requests snapshot the store message revision and channel lifetime on the UI goroutine. Initial completion keeps newer stored versions, appends live arrivals, rejects message-delete tombstones, and discards responses from a deleted/recreated channel lifetime. `PrependMessagesSince` reserves post-request revisions and local echoes, then spends remaining capacity from the fetched oldest edge, preserving live newest-tail arrivals while pagination advances. The local Arikawa `MessageUpdateEvent` unmarshaller records field presence, and app merging changes only present fields. Channel/guild/thread lifecycle handlers post mutations, cascade removals, repair deleted active selections, and notify UI refresh callbacks. All self-ID-dependent gateway classification runs inside posted closures.

All guild/channel/role/thread/archive/forum/directory loader completions also validate store generations and versioned resource gates. Deletion invalidates loaded, pending, exhausted, cursor, and forum-pending state, so an old completion cannot mutate a recreated lifetime or finish its new request. `THREAD_LIST_SYNC` removes only absent cached active threads in the supplied parent scope (or the whole guild when `channel_ids` is nil), preserving archived, out-of-scope, and other-guild threads. Message tombstones are bounded per channel by the history limit; a revision watermark causes any request that overlapped tombstone eviction to be discarded wholesale rather than resurrecting an untracked delete.

## Testing note

Async app tests should use a channel-backed poster and execute posted closures deterministically on the test goroutine. Waiting only for a fake REST method to start or return can race the store mutation even with an immediate poster. Verification for the revision/lifecycle follow-up passed with `go test -race ./internal/app ./internal/store -count=1` and `go test ./... -count=1`.
