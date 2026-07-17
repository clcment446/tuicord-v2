# Discord Client (tuicord) — Phase 2 Plan

> Companion to `PLAN.md` (which covers the from-scratch TUI library, milestones 1–4).
> This plan covers the client itself: full v1 = milestones 5–7. Milestone 8 (images,
> reactions, threads, voice, notifications) is out of scope.

## Context

The from-scratch TUI library (`internal/tui/{text,term,input,screen,layout,tui,widget}`) is
complete and workable. The Discord foundation is partly in place: `internal/discord`
(arikawa v3 user-account session + gateway), `internal/auth` (token resolution:
keyring → `TOKEN` env → prompt), and `internal/keyring` (zalando/go-keyring, service
`tuicord`).

What's missing is the **client itself** — everything that turns "a token" and "a TUI
toolkit" into a usable Discord app: session orchestration, a normalized state store,
Discord-markup rendering, a 4-panel UI, a composer, and login (including QR/remote-auth).

App name is **tuicord-v2**: `cmd/tuicord/`, config at `~/.config/tuicord-v2/config.toml`,
keyring service `tuicord` (already set).

## Reference implementations (on disk, not in this repo)

- **QR login** — `../discordo/internal/ui/login/qr/{model.go,msg.go}`. Hand-rolled
  remote-auth protocol; port the protocol, redraw the QR with our widgets. Same arikawa
  fork we depend on.
- **Discord client wiring** — `../tuicord/internal/discord/` and
  `../discordo/internal/ui/chat/model.go` (session/state/ningen setup, gateway open, READY).

## Existing pieces to reuse (do not rewrite)

- `internal/auth`: `ResolveToken(ctx, Options)`, `ForgetToken`, `KeyringStore`.
  `Options.Prompt PromptFunc` is the hook the login screen fulfills.
- `internal/keyring`: `GetToken/SetToken/DeleteToken` (service `tuicord`).
- `internal/discord`: `NewSession(token)`, `Connect(ctx, s)`. Extend here for handlers.
- `tui.App.Post(fn func())` — the **only** sanctioned bridge from gateway goroutines to
  widgets. Dataflow: gateway handler → mutate store → `app.Post(closure)` → redraw.
- `widget.{Split,List,Viewport,TextInput,Border,Node,Text,Markup,Modal,Image}` and
  `widget.Row/Column`. `Split` already has a drag-resizable divider — use for both sidebars.

## Library gaps to close first (small, additive)

1. **`TextInput.OnSubmit(func(string))`** — fire on Enter (composer, token entry,
   quick-switcher). No Enter hook today.
2. **`widget.ItemList`** — per-item styling (label, badge, style) for channel/guild/
   quick-switcher lists. Keep plain `List` for simple cases.
3. **`tui.WithTheme(Theme)`** — `tui.Option` exists but is empty; add a `Theme` struct of
   `screen.Style`s so config colors flow to widgets.

Each ships with a package/doc comment + `example_test` (text pkg is the DoD template).

## New packages

### `internal/store/` — normalized client state (pure, no I/O, no arikawa in API)
- `Guild{ID, Name, Channels}`, `Channel{ID, GuildID, Name, Kind}`,
  `Message{ID, ChannelID, Author, Content, Timestamp}`, `Member{ID, Nick, Color}`.
- Messages as **ring buffers per channel** (bounded history). Methods: `UpsertGuild`,
  `UpsertChannel`, `AppendMessage`, `ReplaceMessage` (optimistic reconcile),
  `Messages(chID)`, `Guilds()`, `Channels(guildID)`.
- Mutated only inside `app.Post` closures (UI thread) → plain struct, no locks. Document it.
- TDD 80%+ (ring eviction, optimistic replace, ordering).

### `internal/markup/` — Discord markdown → spans (pure)
- Single pass → `[]Span{Kind, Text, ...}`,
  `Kind ∈ {Text, Bold, Italic, Code, CodeBlock, Link, Mention, ChannelMention, Emoji}`.
- Resolve `<@id>`/`<@!id>` → member, `<#id>` → channel, `<:name:id>` → `:name:` (v1 text).
  Resolution via a lookup func backed by `store`.
- Mirror `widget.Markup`'s span model for consistency; this adds the Discord-entity layer.
- TDD 80%+ (nesting, code fences suppress inner parsing, unknown ids degrade).

### `internal/config/` — TOML config
- `~/.config/tuicord-v2/config.toml`; `[layout]` (guilds_width, channels_width, members=auto),
  `[keys]`, `[theme]`. `Load() (Config, error)` with defaults; write default on first run.
- Add dep `github.com/BurntSushi/toml`.

### `internal/app/` — session orchestration (the glue)
- Ties: resolved token → `discord.NewSession` → gateway handlers → `store` → `tui.App`.
- Handlers (READY, GUILD_CREATE, MESSAGE_CREATE…): mutate `store`, then `Post` to invalidate.
- Send: optimistic `AppendMessage` + `Post`, fire REST, on gateway echo `ReplaceMessage`,
  on REST error mark failed (red). Owns which guild/channel is active.

### `internal/ui/` — client composites over the toolkit
- **Login screen** (`ui/login/`): token `TextInput` + **QR panel** in one view. On success →
  `keyring.SetToken` → main UI. Wired as the `auth.PromptFunc`.
  - **QR / remote-auth** (`ui/login/qr.go`): port discordo `qr/msg.go`:
    1. Dial `wss://remote-auth-gateway.discord.gg/?v=2` (gorilla/websocket, browser UA).
    2. `hello` → heartbeat; `rsa.GenerateKey(2048)`.
    3. `init` with `x509.MarshalPKIXPublicKey` base64 pubkey.
    4. `nonce_proof`: RSA-OAEP decrypt nonce, SHA-256, reply base64url.
    5. `pending_remote_init`: `fingerprint` → QR `https://discord.com/ra/<fingerprint>`.
    6. `pending_ticket`: decrypt payload → "Check your phone!" + username.
    7. `pending_login`: `ticket` → close WS → `client.ExchangeRemoteAuthTicket(ticket)`
       (exists in arikawa fork `api/remote_auth.go`) → RSA-OAEP-decrypt `encrypted_token`.
  - **QR rendering**: `github.com/skip2/go-qrcode` (add dep) → half-block glyphs
    (`▀`/`▄`/space, fg/bg per pixel pair) via our `screen.Region`. Goroutine → `app.Post`.
  - Deps: `github.com/gorilla/websocket`, `github.com/skip2/go-qrcode`. Crypto = stdlib.
- **Main layout** (`ui/main.go`): 4-panel `Split` tree `guilds | channels | chat | members`.
  Both sidebars drag-resizable. Members auto-hides under 120 cols (`layout.Node.HideBelow`).
  Always-live composer at bottom of chat column.
- **Panels**: guilds/channels use `ItemList`; chat is a `Viewport` rendering `store.Messages`
  through `internal/markup`; composer is `TextInput` with `OnSubmit`.
- **Keys**: Tab cycles panels, Esc → composer, Ctrl+K quick-switcher (`Modal` + filtered
  `ItemList`). From `config[keys]`. No vim modes in v1.

### `cmd/tuicord/main.go`
- `config.Load` → `tui.App` (`WithTheme`) → `auth.ResolveToken` (prompt = login screen) →
  `internal/app` orchestrator → `app.Run(mainUI)`.

## Build order (mirrors milestones 5–7)

1. **Library gaps**: `TextInput.OnSubmit`, `ItemList`, `tui.WithTheme` (+ example_test each).
2. **M5 skeleton**: `config`, `store`, `app` (READY + GUILD/CHANNEL/MESSAGE, read-only),
   `ui` main layout + panels, `cmd/tuicord`. Token login first. Result: browse + read.
3. **QR login**: `ui/login/qr.go` + rendering; add gorilla/websocket + skip2/go-qrcode.
4. **M6 composer + markup**: `internal/markup`; composer send (optimistic + reconcile + retry).
5. **M7 comfort ring**: unread/mention badges, Ctrl+K quick-switcher, help overlay, themes.

## Definition of done

Each new package: package/doc comment, `example_test`, 80%+ on pure cores
(`store`, `markup`, `config` parse), `examples/` entry where sensible, `go vet` clean.
Survives tmux / 80×24 / `NO_COLOR` / CJK+emoji paste (all width math via `internal/tui/text`,
never `len(s)`/`[]rune` in draw code).

## Verification

- **Unit**: `go test ./internal/store/... ./internal/markup/... ./internal/config/...` 80%+.
- **Login (token)**: `TOKEN=<t> go run ./cmd/tuicord` → lists populate, messages stream.
- **Login (QR)**: no token → QR shown → scan on mobile → approve → token saved to keyring.
- **Send**: type + Enter → optimistic append → reconcile on echo; kill net → red retry.
- **Layout**: <120 cols hides members; drag dividers; `NO_COLOR=1`, 80×24 tmux clean.
- **Headless**: reuse `examples/internal/demo` `TUI_EXAMPLE_RENDER=1` to snapshot main layout
  with a fixture store.
