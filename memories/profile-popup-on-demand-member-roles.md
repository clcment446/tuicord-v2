---
name: profile-popup-on-demand-member-roles
summary: The profile card (press u) showed no roles because member role IDs are only learned from live message member payloads, never from REST history.
tags: [#profile, #roles, #members, #popup, #rest, #hydration]
impact: high
commit: 61096ed (dirty)
date: 2026-07-21
created_at: 2026-07-21T00:00:00+01:00
scope: internal/app/app.go, internal/ui/shell.go, internal/ui/profile_popup.go
---

## Problem

The floating profile card (opened with `u` on a message/mention) rendered an
empty Roles section for almost every user.

## Cause

`buildProfileDetails` (`internal/ui/profile_popup.go`) reads roles from
`member.RoleIDs`, but the store only ever learned role IDs from **live**
`MESSAGE_CREATE` events, whose `member` object carries roles and is stored via
`UpsertMember`. REST-loaded history messages only call
`RememberMemberIdentity` (`internal/app/app.go`, history path) with global
identity and no roles, and tuicord never requests guild member lists (no
op 8 `RequestGuildMembers`, no op 14 `GuildSubscribe`). So opening a profile
from any non-live message showed no roles.

## Resolution

Added `App.EnsureMemberDetail(guild, user, done)` which fetches the full guild
member via REST (`memberDetailsLoader.Member` → arikawa `api.Client.Member`,
wired from `sess`) when the store lacks role IDs, upserts it, and runs `done`
on the UI goroutine. `Shell.openProfile` calls it and, on completion, rebuilds
the card with `ProfilePopup.SetDetails` + `Invalidate` (mirrors the async
avatar-fetch refresh). It skips the fetch when roles are already known.

Tests: `TestEnsureMemberDetailFetchesRolesWhenMissing`,
`TestEnsureMemberDetailSkipsFetchWhenRolesKnown`. Full `go test ./...` passes.

Vim's `u` action must also pass the selected `store.Message` identity into the
profile path. Passing only `AuthorID` turns a member-cache miss into a Profile
notice even though `Message.Author` and `AuthorAvatarURL` are already available.
Use those fields as fallbacks when opening and after async detail refresh.

## Notes

Known adjacent issue left untouched: `openProfile` passes `dmGuild=0` to
`buildProfileDetails`, so the shared-DM list on the card is always empty.
See [[named-group-dm-mention-recipients]] for the related DM recipient work.

`TestEnsureMemberDetailFetchesRolesWhenMissing` had a `-race`-only data race
(plain `go test` passed, hiding it). The test waited on `<-fs.memberDone`, but
`fakeSender.Member` closes `memberDone` via `defer` the moment it returns —
i.e. *before* `EnsureMemberDetail`'s goroutine reaches `a.ui.Post` where
`UpsertMember` and the `done` callback actually run. So the post-wait reads
(`refreshed`, `a.store.Member`) raced the goroutine's writes. Fix: synchronize
on the `done` callback (`close(refreshedCh)`) instead of `memberDone` — the
callback runs after the store mutation, giving a real happens-before edge.
General trap: the `*Done` channels in `fakeSender` signal "the stub returned",
NOT "the posted UI mutation completed"; never use them to gate reads of
post-mutation state. Always run `go test -race` on this package.
