---
name: audit-fixes-devclaude-batch
summary: The devclaude branch fixes the regression-audit backlog — all 5 CRITICAL findings, all High items, and most smaller bugs — as 21 scoped, tested commits. Two items remain deferred with reasons.
tags: [#audit, #regression, #devclaude, #store, #gateway, #plugin, #multi-account, #media]
impact: high
commit: 4c2474c
created_at: 2026-07-22T22:30:00+01:00
scope: internal/app, internal/ui, internal/accounts, internal/plugin, internal/store, internal/tui, cmd/tuicord
---

## Done on branch `devclaude` (pushed)

CRITICAL: ordered gateway ingress + read-mark snapshot
([[gateway-ingress-ordering-and-readmark-snapshot]]); account-switch transient
reset; plugin account isolation + rollback/host concurrency
([[plugin-account-and-rollback-isolation]]).

High: GUILD_CREATE channel/role reconciliation; duplicate account-key dedup;
background-account notification switches account before navigating; bounded
desktop-notification dispatch (cap 4); unchanged-member-upsert skips MetaRev bump
(~14x transcript win, see [[chatview-retained-transcript-cache]]); off-UI-thread
composer thumbnail decode; directory load survives partial-endpoint failure and
mid-load generation changes (retry); REST send/edit/delete confirm from their own
response instead of only the gateway echo.

Smaller: MESSAGE_REACTION_REMOVE_EMOJI handler; duplicate MESSAGE_CREATE guard
(Store.HasMessage); category-delete keeps child-channel unread; DM ack (guild 0)
clears synthetic DM badge; mute-setting change refreshes badges; right-click no
longer starts transient-widget drag; unified SSH detection (config.IsSSH) for
video SHM; clipped menus no longer activate undrawn rows; keyboard focus no
longer scrolls one row past its target.

## Deferred (not done)

- Media byte limits (#9): GIF decode already bounds MaxEncodedBytes/MaxMemoryBytes
  and there is a DecodedCacheMaxBytes; the residual "retained frames / plugin
  video stream" gap was ambiguous and platform-specific — left alone rather than
  risk media regressions without a clear repro.
- Pins & collapsed CATEGORIES leak between accounts: these are global fields on
  uistate.State (PinnedGuilds/PinnedChannels/CollapsedCategories). Proper fix is
  per-account, mirroring the folder-collapse migration in
  [[guild-layout-account-key-and-collapse-scope]] — a real schema + migration
  change, deferred as too large/risky for this batch.
