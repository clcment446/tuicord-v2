---
name: matrix-backend-and-goolm-tag
summary: Matrix support is a second backend.Backend implementation (internal/matrixapp) on mautrix v0.29, selected per-account by decoding the keyring value (auth.Decode → Matrix creds JSON vs bare Discord token). The build now REQUIRES `-tags goolm` on every go command (pure-Go olm; without it the build fails on olm/olm.h). Discord path is behavior-preserving behind the extracted internal/backend interface.
tags: [#matrix, #mautrix, #goolm, #e2ee, #backend-interface, #multi-account, #build, #architecture]
impact: CRITICAL
commit: 3ba95b9
date: 2026-07-23
created_at: 2026-07-23T00:00:00+01:00
scope: internal/backend, internal/matrix, internal/matrixapp, internal/auth/creds.go, internal/media/authregistry.go, internal/ui/login_matrix.go, cmd/tuicord/main.go, PKGBUILD, README.md
---

## Build tag (CRITICAL, easy to forget)

- ALL go commands need `-tags goolm`: `go build/test/vet/run -tags goolm ./...`.
  Without it the build fails: `maunium.net/go/mautrix/crypto/libolm` needs the C
  header `olm/olm.h`. `goolm` selects the pure-Go olm implementation — no system
  libolm. cgo is still required (the E2EE SQLite crypto store uses
  mattn/go-sqlite3 via `go.mau.fi/util/dbutil/litestream`, imported blank in
  internal/matrix to register the `sqlite3-fk-wal` driver). PKGBUILD and README
  updated; the plain `go build ./...` in muscle memory now fails.
- Windows: `cmd/tuicord` already did not compile for Windows (lock.go uses
  syscall.Flock, no build tag) — pre-existing, unrelated to Matrix. internal/matrix
  cross-compiles fine (litestream has nocgo.go), so E2EE just won't have a driver
  registered on a CGO_ENABLED=0 build (runtime, not compile).

## Architecture

- The protocol seam is `internal/backend.Backend` (extracted first, commit ef… the
  refactor commit). `*app.App` (Discord) and `*matrixapp.App` (Matrix) both
  implement it. UI/accounts/plugins depend only on backend. See
  [[ningen-migration-and-multi-account]] for the pre-existing multi-account model
  the Matrix backend plugs into unchanged.
- Discord-only calls that reference app/arikawa types (SubmitCommand,
  SubmitComponent, LoadCommands, SearchGIFs) are NOT on the interface; Shell holds
  a `discord *app.App` set by type assertion and nil-guards them. Everything else
  (roles/channels/forum/threads/sticker) is on the interface and matrixapp no-ops it.
- Account routing: keyring stores a bare token (Discord) or a JSON
  `auth.Credentials{Protocol:"matrix",...}` blob. `newBackendFromValue` in
  cmd/tuicord decodes and builds the right backend. uistate/config Account gained
  `Protocol`/`Remote` (empty = discord, back-compatible).

## Matrix specifics (internal/matrixapp)

- Sync: mautrix DefaultSyncer dispatches synchronously on one goroutine; handlers
  build store mutations and `ui.Post` them (same FIFO/single-writer discipline as
  the Discord gateway handlers). Connect owns the reconnect/backoff loop around
  SyncWithContext. cryptohelper (installed in matrix.New) auto-decrypts
  m.room.encrypted and re-dispatches plaintext, and auto-encrypts on send.
- IDs: string mxids interned to uint64 via a per-account persisted table
  (idmap.json). Rooms/users persist (uistate layouts are keyed by uint64); event
  IDs are ephemeral in a disjoint high range (>= 1<<48), never persisted.
- Placement: spaces (m.room.create type=m.space) → guilds; m.space.child →
  child placement; m.direct → DirectMessagesGuildID; leftovers →
  UngroupedRoomsGuildID ("Rooms"). Unread from sync unread_notifications.
- Media: `internal/media` gained a FetchAuthorizer hook; matrixapp registers one
  that adds the Bearer header for its homeserver's authenticated-media URLs and
  decrypts encrypted attachments (keys stashed per download URL at convert time).
  Discord URLs are unclaimed → unchanged.

## Deferred (not in v1)

- OIDC/MAS device-authorization login (password + access-token paste work;
  matrix.org now requires OIDC so password login there will fail — needs the
  device grant). Refresh-token wiring is stubbed (SaveNewToken set, but
  SetToken-equivalent for a stored refresh token is not wired).
- Threads (LoadActiveThreads/CreateThreadFromMessage are no-ops), SAS/emoji
  device verification, recovery-key restore (`;matrix-recover`).
- Not live-tested against a real homeserver (no account available in-session);
  verified by build/vet/`go test -race -tags goolm` + a no-panic TTY startup smoke.
