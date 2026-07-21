---
name: dm-recipient-avatar-and-member-list
summary: DM/group participants live on channel.Recipients (guild=0), not Store.Members; the recipient Member must be built with AvatarURL/Username, and the member sidebar must fall back to Recipients like the @-menu does.
tags: [#dm, #group, #members, #avatar, #profile, #sidebar, #recipients]
impact: normal
commit: 460ae39 (dirty)
date: 2026-07-21
created_at: 2026-07-21T00:00:00+02:00
scope: internal/app/convert.go, internal/ui/main.go, internal/ui/profile_popup.go
---

## Two DM bugs (both pre-existing data, not rendering)

DMs/group DMs have `guild == 0` and `Kind == ChannelDM` (no separate group
kind); their participants are `Channel.Recipients` / `RecipientIDs`, populated in
`convertChannel` (`convert.go`). Guild members (`Store.Members(guild)`) are empty
for them.

1. **No profile picture in the DM user overlay.** `convertChannel` built each
   recipient `store.Member` with only `ID`+`Name` — no `AvatarURL`/`Username`. So
   `openProfile`'s fallback (`memberForContext` → `Store.ChannelRecipient`) got an
   empty `AvatarURL` and `fetchProfileAvatar` returned early. Fix: build the
   recipient from the `discord.User` with `AvatarURL: recipient.AvatarURL()` and
   `Username: recipient.Username`. (`buildProfileDetails` only fills AvatarURL from
   guild members / the fallback, so the recipient must carry it.)

2. **Member sidebar empty for DMs/groups.** `MainView.refreshMembers(guild)` read
   `Store.Members(guild)` only. Fix: when the active channel is a `ChannelDM`, use
   `channel.Recipients` (same fallback `inline_picker.go` `@`-menu already uses).
   Also call `refreshMembers(ch.GuildID)` from `updateChannelChrome` so DM→DM
   switches refresh (guild switches refresh separately, but DM→DM shares guild 0
   and previously never re-ran). See [[named-group-dm-mention-recipients]],
   [[dm-mention-recipients-and-structured-hot-switch]],
   [[profile-popup-on-demand-member-roles]].
