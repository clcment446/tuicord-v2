---
name: guild-groups-and-dm-ordering
summary: Server groups disappeared because IDENTIFY requested protobuf user settings that the arikawa fork cannot decode; clear UserSettingsProto so legacy guild_folders arrive. DM channels need last_message_id ordering because their positions are all zero.
tags: [#discord, #gateway, #guild-folders, #server-groups, #direct-messages, #ordering, #arikawa]
impact: high
commit: dirty
date: 2026-07-22
created_at: 2026-07-22T00:00:00+03:00
scope: internal/discord/client.go, internal/app/convert.go, internal/store/store.go, internal/store/ordering.go, internal/ui/main.go
---

## Problem

The server rail rendered every guild as a flat list even though the store and UI
already supported expandable guild folders. Direct Messages appeared at the
bottom, its panel was titled Channels, and DM conversations were oldest-first.

## Cause

`clientCapabilities = 16381` includes `gateway.UserSettingsProto`. Discord then
omits the legacy `user_settings` READY field and uses protobuf settings updates.
The current arikawa fork defines that capability but only decodes legacy
`UserSettings` and `UserSettingsUpdateEvent`, so `guild_folders` never reached
the store.

Private channels normally have the same position. `Store.Channels` therefore
fell back to ascending channel ID, which approximated oldest-created first.

## Resolution

- Clear `gateway.UserSettingsProto` from IDENTIFY until protobuf settings are
  actually decoded. Do not add a startup REST settings request; preserve the
  startup request limits described in [[startup-api-burst-and-instance-safety]].
- Keep Discord's `last_message_id` on store channels and advance it when a
  gateway message or confirmed local echo arrives.
- Sort only the Direct Messages view by last message ID descending, with channel
  ID descending as the empty-history tie-breaker.
- Move the synthetic Direct Messages guild row to the top, title its channel
  panel Direct Messages, and render unnamed real guild folders as Group.

## Verification

Focused app, Discord, store, and UI tests pass. The command binary builds. The
full suite requires localhost sockets for unrelated captcha and plugin tests,
which the restricted sandbox blocks.
