---
name: ningen-migration-and-multi-account
summary: Adopt ningen by wrapping the existing user-token session (state.NewFromSession + ningen.FromState) and KEEP a.handle.Connect (promoted from session) not ningen.Open, so the reconnect loop + 4004 surfacing stay while ningen states still populate. Multi-account is one process, N accounts sharing one tui runtime, driven by accounts.Manager via a Surface interface so accounts never imports ui.
tags: [#ningen, #arikawa, #multi-account, #gateway, #user-session, #architecture, #readstate]
impact: high
commit: dirty
date: 2026-07-20
created_at: 2026-07-20T00:00:00+01:00
scope: internal/discord/client.go, internal/app/app.go, internal/accounts, internal/ui/main.go, internal/ui/shell.go, internal/config/config.go, cmd/tuicord/main.go
---

## ningen migration (phase 1 of the RPC-split plan)

- ningen wraps arikawa, it does not replace it. `discord.NewNingen(token)` builds
  the SAME user-token session as `NewSession` (custom IdentifyProperties,
  clientCapabilities=16381, direct `wss://gateway.discord.gg/`, browser-shaped
  `sess.Client`), then `state.NewFromSession(sess, defaultstore.New())` →
  `ningen.FromState(st)`. `WrapSession(sess)` does the wrap for tests that hold a
  fake session (use `session.New("")`, NOT `&session.Session{}` — the latter has a
  nil handler and panics in state hookSession).
- Go module: pin ningen to the **v3 branch pseudo-version** (…20260306…5a08d3a709b4),
  NOT the `v3.0.0` tag — the tag requires a 2022 arikawa and clashes with the
  vendored 2026 copy. The `replace github.com/diamondburned/arikawa/v3 =>
  ./third_party/arikawa` makes ningen use the vendored, patched arikawa
  automatically, so the MESSAGE_UPDATE field-presence patch stays applied for free.
- `App.handle` is now `*ningen.State`. `a.handle.AddHandler` resolves to ningen's
  own Handler (depth-1 embed), which forwards every event AFTER ningen's states
  update — so ReadState is fresh when app handlers run.
- CRITICAL: `Connect` is left UNCHANGED (`a.handle.Connect(ctx)`). It is promoted
  from the embedded session and keeps the reconnect loop + fatal-close (4004)
  reporting. `ningen.Open` returns after READY and does NOT block/reconnect. ningen
  states populate via ningen's synchronous handler regardless of Open vs Connect,
  so Connect is correct. Do not switch to ningen.Open without adding a Wait/blocking
  reconnect equivalent.
- clientCapabilities=16381 is a superset of ningen's required ReadState caps
  (VersionedReadStates|DedupeUserObjects|NonChannelReadStates… = 253); 253 & 16381
  == 253, so no capability change is needed.
- `store.Store` is KEPT as the synchronous render model (UI reads it every frame in
  Draw). ningen's cabinet is additive; only `ReadState.TotalMentionCount()` is used
  (for the account mention badge). `App.Unread()` combines that with the store's
  `TotalUnread`/`TotalPings` for the unread dot.

## Multi-account (single process)

- `internal/accounts.Manager`: one process, N accounts, each with its own
  ningen+store+app, all posting onto the ONE shared `*tui.App`. Lazy connect: only
  the active account connects at launch; others connect on first `Switch` and stay
  connected. Callback routing: panel refresh only for the active account; badges +
  notifications for every connected account.
- `accounts` MUST NOT import `internal/ui` (would cycle, since the selector widget
  lives in ui). It drives the UI through a `Surface` interface; the concrete adapter
  (`uiSurface`) lives in `cmd/tuicord/main.go`, which imports both. The selector
  widget takes a plain `onSelect func(int)` so ui stays accounts-free.
- The launch account is built BEFORE the Manager (its app/store back MainView/Shell,
  and its interactive login must run before the tui loop owns the terminal), then
  handed in via `Seed{App, Store, Ning}`; `ensureBuilt` skips construction and just
  wires callbacks. Lazy accounts are built via `Build(key)` (keyring-only, no prompt).
- `config.Accounts` is held by POINTER on Config (like Plugins) so Config stays
  comparable (`cfg != Default()` in tests). Tokens are NEVER in config — only a
  keyring key + label + id. Legacy single token (`keyring.LegacyTokenKey = "token"`)
  is migrated into a one-entry registry; per-account tokens use `KeyringStore{Key}`
  / `keyring.GetTokenFor(key)`.
- UI: account selector is a `widget.ItemList` to the LEFT of the composer — the
  composer row became `widget.NewSplit(accountBorder, composerBorder)` (left-to-right
  is the Split default). `mv.composerNode` now points at that row's node.
  `GuildsWidth 4→3`, `ChannelsWidth 24→20`. Rebind on switch:
  `MainView.SetActiveAccount(app)` (re-points mv.app + `ChatView.SetSource`) and
  `Shell.SetActiveAccount(app)` (pointer swap; post/tryPost are shared-runtime).

## Deferred / known limits

- The gRPC daemon/TUI split (auto-spawn embedded daemon holding the lock, TUIs as
  clients) is a LATER phase — per-account app.App + store.Store are kept
  self-contained so they can move server-side. Decisions locked for it: auto-spawn
  embedded daemon, gRPC transport.
- Plugins bind to the LAUNCH account's orchestrator and do NOT follow account
  switches in this phase.
- Account unread "dot" uses the account's own store (a background account's
  activeChannel is stale, minor inaccuracy); mention count is authoritative from
  ningen ReadState.
- Not verified live end-to-end here (needs a real TTY + Discord auth; connecting
  real tokens risks flagging). Verified: build (both modules), vet, `go test -race`
  on accounts/app/store/config/ui, and a startup smoke run reaching the TTY stage
  with no panic. Related: [[startup-api-burst-and-instance-safety]],
  [[discord-gateway-store-lifecycle-correctness]], [[login-auth-failures]].
