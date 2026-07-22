---
name: chatview-retained-transcript-cache
summary: ChatView keeps a compact flattened transcript keyed by per-channel message revision and presentation generations; rendered lines carry one-based message handles into a retained snapshot instead of copying store.Message per line.
tags: [#chat, #rendering, #cache, #performance, #allocations, #viewport, #store]
impact: critical
commit: pending
date: 2026-07-22
created_at: 2026-07-22T12:00:00+03:00
scope: internal/store/ring.go, internal/store/store.go, internal/store/mutation.go, internal/ui/chatview.go, internal/ui/chat_anchor.go, internal/ui/componentview.go
---

## Problem

The per-message body cache still rebuilt a flattened transcript on every frame.
At the 200-message history limit an unchanged render copied about 6.1 MB, and
Draw additionally copied the store three times and rebuilt focus maps across
every line. Moving focus to each new latest message also invalidated every body.

## Resolution

Each store ring now carries the latest visible channel revision. `MsgsInto`
copies into ChatView-owned capacity, while `LastMsg` and `MsgEdges` avoid full
snapshots for focus and scroll bookkeeping. Every successful message mutation,
including removal, advances the ring revision.

ChatView retains the flattened transcript and reuses it only when channel,
message revision, metadata revision, component epoch, style generation, media
epoch, width, and dynamic-state eligibility still match. Loading spinners,
animated media, and animated role gradients keep the transcript unstable.

Rendered lines store a one-based `msg` handle into the retained message slice.
Zero means no message. Cached bodies always keep zero handles; transcript
assembly stamps handles only onto copied lines. Anchor, focus, selection, and
mouse paths resolve messages through `msgAt`.

Focus movement invalidates only the previous and next focused bodies because
only those bodies change component shortcut presentation. Broad presentation
changes clear the body cache. Source, style, and media rebinding are broad
invalidations. Focus indexes rebuild only when the transcript generation moves.

## Invariants

- A new store message mutator must update both the message revision and ring revision.
- A transcript fast hit must not call `MsgsInto` or rebuild focus metadata.
- Never stamp a store.Message value into each chatLine; use the retained handle.
- Cached loading or animated bodies would freeze visual state and are forbidden.
- Component action keys are computed when their body renders, not on every focus-index rebuild.
- Generic body invalidation clears the map; focused-message invalidation deletes only the affected identities.

## Verification

`internal/store/rev_test.go`, `internal/ui/chatview_cache_test.go`, the ChatView
equivalence and anchor suites, and `go test ./... -count=1` cover the mutation
and viewport behavior. Benchmarks are in `internal/ui/chatview_bench_test.go`.
