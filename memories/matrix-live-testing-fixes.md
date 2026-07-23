---
name: matrix-live-testing-fixes
summary: First live homeserver test of the Matrix backend surfaced two bugs. (1) Notification flood on every launch — onMessage fired OnIncomingMessage for the whole initial-sync backlog; gated with an atomic caughtUp set from the sync token (empty == initial/reconnect catch-up) in onSync, which mautrix runs before timeline dispatch. (2) Read-only composer in every non-DM room — the UI read-only gate is store.ChannelCan(PermSendMessages), and synthetic Matrix guilds had no @everyone role, so it denied sending; ensureGuildRow now installs a permissive @everyone role (id == guild id). "Can't read" old messages is E2EE (no key-backup restore on a fresh device), not a bug.
tags: [#matrix, #matrixapp, #notifications, #permissions, #readonly, #e2ee, #sync, #live-test]
impact: high
commit: 05a1e6e (dirty)
date: 2026-07-23
created_at: 2026-07-23T19:17:41+01:00
scope: internal/matrixapp/sync.go, internal/matrixapp/app.go, internal/matrixapp/rooms.go
---

## 1. Notification flood on every launch

`onMessage` called `a.onIncoming(msg)` for every non-self message with no
initial-sync guard. mautrix's initial `/sync` (empty `since`) replays each
room's recent timeline, so every backlog message raised a desktop
notification on each launch.

Fix: `App.caughtUp atomic.Bool`. In `onSync`, `a.caughtUp.Store(since != "")`.
`onSync` (an `OnSync` listener) runs BEFORE per-event dispatch in mautrix
`DefaultSyncer.ProcessResponse` (verified in mautrix@v0.29.0 sync.go — sync
listeners loop first, then `processSyncEvents`), so the gate set there governs
that same batch's messages. `onMessage` captures `notify := !fromSelf &&
a.caughtUp.Load()` ON THE SYNC GOROUTINE (not inside Post) so a later sync
flipping the gate can't retroactively notify the initial batch. Tracking the
token (not a one-shot "seen first sync") also keeps a from-scratch reconnect
catch-up quiet.

## 2. Read-only composer in every non-DM Matrix room ("can't send")

`MainView.channelReadOnly` → `composer.SetReadOnly`. Its gate is
`store.ChannelCan(guild, self, channel, PermSendMessages)` (Discord's
permission model). It skips the check for DMs (`DirectMessagesGuildID`) and
`GuildID==0`, so DMs were writable — but rooms in a space or the "Rooms" guild
(`UngroupedRoomsGuildID`) hit the check, and synthetic Matrix guilds had no
`@everyone` role, so `MemberPermissions` returned 0 → denied → read-only.

Fix: `ensureGuildRow` now calls `ensureWritableRole(guild)`, upserting an
`@everyone` role (`ID == RoleID(guild)`, which `MemberPermissions` reads as the
baseline) with participation bits only —
`PermViewChannel|SendMessages|AddReactions|SendMessagesInThreads|CreatePublicThreads`
— NOT management bits, so Discord moderation UI stays hidden. Matrix power
levels are still not enforced (any joined room is writable regardless of
`events_default`); acceptable for now.

## 3. E2EE broken: plaintext sends + "olm account not marked as shared"

Symptom: toast "olm account is not marked as shared, but there are keys on the
server"; own messages (sent from Element) don't decrypt; other clients show
tuicord's messages as "Not encrypted".

Root cause: the user pasted their **Element access token**. `matrix.LoginToken`
sets `DeviceID = who.DeviceID` — i.e. tuicord adopts Element's device — but has a
brand-new, empty local olm account. mautrix `cryptohelper.Init` →
`verifyDeviceKeysOnServer` sees the server already has keys for that device while
the local account's `Shared==false` → returns that error. Two clients cannot
share one device's E2EE identity. Init fails AFTER creating the machine but
BEFORE registering its sync handlers (mautrix@v0.29.0 cryptohelper.go:92-116), so
the crypto state store is never populated → `Client.SendMessageEvent`'s
`StateStore.IsEncrypted` returns false → **messages are sent as plaintext into
encrypted rooms**. `Connect` only `reportError`d the failure and continued.

Real fix (deferred, big): tuicord must log in as its OWN device — OIDC/MAS device
authorization for matrix.org, or password login on a homeserver that allows it,
or a token freshly minted for a new device. Historical Element messages then
still need key-backup restore / SAS verification ([[matrix-backend-and-goolm-tag]]).

What was fixed here (safety + clarity, NOT making E2EE work):
- `App.cryptoReady atomic.Bool` set only when `StartCrypto` succeeds.
- `roomInfo.encrypted` tracked from a new `m.room.encryption` handler
  (`onStateEncryption`), independent of the crypto state store (which is empty
  when crypto failed).
- `SendToChannel`/`SendFiles` call `blockedByEncryption`: if the room is
  encrypted and `!cryptoReady`, refuse the send (surface `errEncryptionUnavailable`)
  instead of leaking plaintext.
- `cryptoSetupError` maps the olm-not-shared / mismatching-identity errors to an
  actionable "sign in with a device of its own" message.

Undecryptable *incoming* messages (missing keys on a fresh device) render the
`onDecryptError` placeholder "🔒 Unable to decrypt this message" — working as
intended; also needs key backup / verification.

Tests: `internal/matrixapp/notify_writable_test.go` (initial-sync silence, live
notify, self-suppression, gate tracking, writable guild) and
`crypto_guard_test.go` (encrypted-send refusal, gate states, error mapping).
Related: [[matrix-backend-and-goolm-tag]], [[ningen-migration-and-multi-account]].
