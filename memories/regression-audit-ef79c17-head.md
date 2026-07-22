---
name: regression-audit-ef79c17-head
summary: The ef79c17..d399cc0 regression cluster came from unconditional explicit-focus anchoring, unseeded read-state caches, stale index-based drags, action-time key defaults, and popup hit-testing that diverged from drawing.
tags: [#regression, #chat, #read-state, #vim, #modal, #drag, #multi-account]
impact: critical
commit: c84f09a
created_at: 2026-07-22T19:30:00+01:00
scope: internal/ui, internal/app, internal/store, internal/tui, internal/accounts, internal/uistate
---

## Audit range

The inclusive range `ef79c17e3d31ed0f729b7944098eb0e49b6293ec..d399cc0`
was reviewed commit-by-commit and correlated with GitHub issues #46-#50.

## Root causes and invariants

- Chat explicit focus is not itself proof that the viewport should be anchored.
  Anchor appended content only while actually reading above the live edge, and
  never overwrite an offset changed by user navigation on an unchanged draw.
- Event-derived read-state caches must be reset/seeded on READY, updated by both
  ningen read events and `CHANNEL_UNREAD_UPDATE`, and pruned on channel/thread/
  guild deletion. Effective mute/permission semantics beat raw unread bits.
- `LastMessageID` is a monotonic activity watermark. Preserve pre-hydration
  observations and never let stale channel upserts move it backward.
- Drag callbacks using row indices are valid only for the item generation that
  started the drag. Replacing rows must invalidate capture before drawing/drop.
- Empty configured Vim bindings mean disabled. Defaults belong in configuration
  construction, not action-time fallback. Key matching must support enhanced
  Shift reporting and named non-rune keys consistently across widgets.
- Popup hit-testing must share geometry with drawing. Opaque overlays consume
  unsupported pointer events, and later-drawn toasts receive pointer priority.
- Guild layouts and collapsed local folder IDs are account-scoped and must use
  the stable account registry key before READY, not Discord user ID zero.

## Verification

Regression tests cover draw-after-navigation, incoming/optimistic appends,
reaction actions, READY/read deletion lifecycle, pre-hydration unread events,
stale channel activity, drag invalidation, named/disabled keys, modal geometry,
popup click-through, account-state migration, and SSH defaults. Full tests,
selected race tests, `go vet ./...`, and `git diff --check` passed.
