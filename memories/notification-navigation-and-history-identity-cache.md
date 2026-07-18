---
name: notification-navigation-and-history-identity-cache
summary: Incoming-message navigation and mention fallback require preserving actions through notification fallback and seeding authors from REST history as well as gateway events.
tags: [#notifications, #navigation, #mentions, #identity-cache, #history]
impact: high
commit: 11afe37 (dirty)
date: 2026-07-17
created_at: 2026-07-17T21:02:47Z
scope: internal/ui/shell.go; internal/app/app.go
---

## Problem

`internal/ui/shell.go:1019-1052` initially made only focused in-app incoming-message toasts actionable. A desktop-notification failure fell back to the generic, non-actionable toast. `internal/app/app.go:1102-1175` cached absent message members only for live `MESSAGE_CREATE`, leaving mention names unresolved in REST-loaded and paginated history.

## Cause

The fallback route discarded the source `store.Message`, and the REST history paths converted messages only for transcript storage rather than also using their authors as identity cache inputs.

## Resolution

Route both focused and failed-desktop notifications through `showIncomingMessageToast`, which retains the channel activation callback. In both history pagination paths, call `RememberMemberIdentity` with a member built from each Discord message author when the target channel belongs to a guild. `RememberMemberIdentity` updates identity fields without overwriting nick, guild avatar, or role IDs.

## Notes

System desktop notifications themselves still depend on platform notifier capabilities; the in-app fallback is the reliable click-to-channel route. Focused verification passed with `go test ./internal/store ./internal/config ./internal/ui -count=1`; `go build ./internal/app ./cmd/tuicord` also passed. The full app test package remains blocked by pre-existing `autobot_local_test.go` references to removed APIs.
