---
name: guild-layout-account-key-and-collapse-scope
summary: Guild layout identity must use the stable account registry key because SelfID is zero before READY; folder-collapse IDs belong inside that same per-account layout because local negative IDs collide across accounts.
tags: [#multi-account, #guild-layout, #guild-folders, #ui-state, #migration]
impact: high
commit: dirty
date: 2026-07-22
created_at: 2026-07-22T19:13:05+01:00
scope: internal/uistate/uistate.go, internal/ui/guild_groups.go, internal/ui/main.go
---

## Invariant

Persist and resolve guild layouts by `AccountKey` first. Keep `AccountID` only as
backward-compatible metadata and as a fallback for adopting old ID-only layouts;
never use ID zero as an identity. This keeps two accounts saved before READY from
replacing each other's layouts and lets a keyed layout survive later SelfID
hydration.

Folder collapse is layout state, not global UI state. Store `CollapsedFolders`
on each `GuildLayout`, and make every MainView read/toggle pass the active
registry key plus the best currently known ID. This is especially important for
locally created folder IDs (`-1`, `-2`, ...), which intentionally repeat across
accounts.

## Migration

On load, associate old nonzero-ID layouts with matching registry keys. Associate
an old anonymous ID-zero layout with the active registry account, the only owner
legacy state can identify. Copy legacy top-level `collapsed_folders` into known
account layouts, clear the top-level field, and rerun migration before save so
accounts seeded from older config after load still adopt anonymous state.

A collapse-only layout must not override Discord's server folder ordering:
`GuildLayout` reports a custom group layout only when `Groups` is non-nil.
