# tuicord — PLAN #4: Interaction Layer

> Companion to `PLAN_PHASE_3.md` (rich content). Scope: acting *on* things —
> pickers, context menus, profile/role popups, guild management tabs, and
> list ordering (folders, positions, pins). Depends on PLAN #3's clickable
> spans + `internal/media` (emoji/gif/sticker thumbnails in the picker).

## Context

After PLAN #3 the client renders everything but can only *send text*. The
mouse path (`tui.App.handleMouse` → hit test → `Handle`) already routes
`ButtonRight`, `Modal` supports drag + z-order, and `widget.Button` exists.
What's missing is a reusable popup/menu layer and the REST surface for
message/role/channel management.

## Existing pieces to reuse

- `widget.Modal` (z-ordered, draggable) — pickers and profile popups are
  modals. `ui/quickswitch.go` is the pattern for "modal + filtered ItemList".
- `widget.ItemList` (per-item label/badge/style) — menu bodies, picker grids.
- PLAN #3 `Action` spans in ChatView — right-click hit-testing reuses the
  same y→message map; clicking a mention/role reuses span actions.
- `internal/media` cache — picker thumbnails, profile avatars.
- `ui/toast.go` — every REST failure surfaces as a toast, never a crash.
- `app.Post` discipline — unchanged: REST on goroutines, results posted.

## Library gaps to close first (you implement)

1. **`widget.Menu`** — a lightweight popup list anchored at (x, y): items
   `{Label, Key hint, Danger bool, Disabled bool, OnSelect}`, separator
   support, Esc/click-outside dismiss, submenu optional (flat v1 is fine).
   Clamps to screen edges. This is *the* deliverable of this phase's library
   work — pickers, context menus, and guild tabs all sit on Modal+Menu.
2. **`widget.Tabs`** — horizontal tab strip + child swap (guild settings
   modal, picker's Emoji/GIF/Sticker tabs).
3. **Clipboard** — OSC 52 write helper in `term` (`term.CopyToClipboard`),
   with `xclip`/`wl-copy` fallback. Needed by "Copy message ID".

## Features

### 1. Emoji / GIF / Sticker picker (with fake-nitro support)

- Trigger: `Ctrl+E` in composer (config `[keys] picker`). Modal with
  `Tabs[Emoji | GIF | Sticker]`, a search `TextInput`, and a grid
  (`ItemList` in grid mode or a small purpose-built grid widget).
- **Emoji tab**: unicode emoji from a static table (group + names, generate
  from Unicode CLDR data into a Go file) + custom emojis from every guild
  (`store` gains `Emojis(guild)`; synced from GUILD_CREATE + GUILD_EMOJIS_UPDATE).
  Insert: unicode → literal char; own-guild custom → `<:name:id>`.
- **Sticker tab**: guild stickers (GUILD_STICKERS_UPDATE) + recent. Send:
  `sticker_ids` on the message create payload.
- **GIF tab**: search Discord's own tenor proxy
  (`GET /gifs/search?q=&provider=tenor` — no API key needed on a user
  account; arikawa fork may need a tiny endpoint wrapper). Grid of thumbs via
  `internal/media`. Send: post the gif URL as content (that *is* how Discord
  sends gifs).
- **Fake nitro**: when the selection isn't usable (animated/other-guild emoji,
  other-guild sticker, without nitro): insert the CDN URL instead —
  emoji → `https://cdn.discordapp.com/emojis/<id>.<png|gif>?size=48&name=<name>`,
  sticker → sticker CDN URL. PLAN #3's classifier renders these back as the
  real thing. Config `[nitro] fake = true`. Detect actual nitro from READY
  user premium_type to prefer real sends when possible.

### 2. Message context menu (right-click)

- Right-click a message in ChatView → `widget.Menu` at cursor:
  - **Reply** / **Reply (no mention)** — composer enters reply mode (banner
    above input: `↩ replying to @name [mention: on]`); send sets
    `message_reference` and `allowed_mentions.replied_user=false` for the
    no-mention variant. Esc cancels.
  - **Edit** (own messages only) — composer pre-filled, `PATCH
    /channels/<c>/messages/<id>`; banner `✎ editing`. MESSAGE_UPDATE handler
    (PLAN #3) reconciles.
  - **Delete** — `DELETE …/messages/<id>` after a confirm menu entry
    ("Delete — click again"). **Force delete** — same endpoint but shown for
    messages we don't own when we hold MANAGE_MESSAGES (permission check from
    role permission bits — store roles already carry what's needed; add
    `Permissions` field in `store.Role` + pure `HasPermission` calc, TDD).
  - **Pin / Unpin** — `PUT/DELETE /channels/<c>/pins/<id>`; label reflects
    current state (needs `Pinned` flag on `store.Message`, patched by
    MESSAGE_UPDATE + CHANNEL_PINS_UPDATE).
  - **Copy message ID** — OSC 52 (gap 3) + toast "copied".

### 3. Click-through popups

- **User mention click → profile modal**: avatar (media cache), display name,
  username, colored role list, joined dates from
  `GET /users/<id>/profile` (user-account endpoint; fall back to member data
  offline). Read-only v1.
- **Role click → role options modal**: role name (colored), and CRUD:
  rename, change color(s), hoist, mentionable toggles → `PATCH
  /guilds/<g>/roles/<id>`; **Create** (`POST /guilds/<g>/roles`), **Delete**
  (with confirm). Entries disabled without MANAGE_ROLES. Form = labeled
  `TextInput`s + `Button`s in a Modal.

### 4. Guild right-click → management tabs

- Right-click a guild in the sidebar → Menu → "Server settings…" opens a
  Modal with `Tabs`:
  - **Channels tab**: `ItemList` of channels grouped by category; rename,
    delete, create (`POST/PATCH/DELETE /guilds/<g>/channels`), reorder via
    move up/down entries (`PATCH /guilds/<g>/channels` position batch).
  - **Roles tab**: role list ordered by position; same CRUD as the role modal
    plus reorder (`PATCH /guilds/<g>/roles` position batch).
- All writes optimistic-free (wait for gateway echo — CHANNEL_UPDATE /
  GUILD_ROLE_UPDATE already handled); failures → toast.

### 5. Ordering: folders, positions, pins

- READY carries `user_settings.guild_folders` (folders with color + guild
  ids) — store gains `GuildFolders`; the guild sidebar renders folders as
  collapsible headers, guilds in folder order (fallback: current order).
  Track USER_SETTINGS/USER_GUILD_SETTINGS update events.
- Channels: already sorted by `Position`; add **category grouping**
  (`ChannelKind` category as non-selectable headers, children indented,
  collapse on click — collapsed set persisted in config state file
  `~/.local/state/tuicord/ui.toml`, not the main config).
- **Pinned/favourite channels & guilds** (client-side): context-menu entry
  "Pin" on channels/guilds → pinned section on top of each list; persisted in
  the same state file. Pure sort funcs (`orderGuilds(folders, pins, base)`)
  — TDD.

## Build order

1. **Library gaps**: `Menu`, `Tabs`, OSC 52 clipboard (+ example_tests).
2. **Permissions**: `store.Role.Permissions` + `HasPermission` (TDD) — gates
   half the menu items, do it early.
3. **Message context menu** (reply/edit/delete/pin/copy) + composer
   reply/edit modes. Highest daily value.
4. **Ordering** (folders, categories, pins) — pure + visual, no new REST.
5. **Picker** (emoji → sticker → gif tabs, fake-nitro insert last).
6. **Profile + role popups**, then **guild management tabs** (biggest REST
   surface, builds on the role modal).

## Definition of done

House rules hold: package comments, example_tests, 80%+ on pure cores
(permission calc, ordering, picker filtering, fake-nitro URL building),
`go vet` clean, all REST off-thread with toast-on-error, every popup
dismisses on Esc and click-outside, everything reachable by keyboard too
(menus are lists — arrows + Enter), 80×24 safe (popups clamp).

## Verification

- **Unit**: permission bits, guild/channel ordering, fake-nitro URL round-trip
  (build URL → PLAN #3 classifier recognizes it), reply payload building.
- **Live**: reply w/ and w/o mention, edit, delete own, force-delete as mod,
  pin/unpin, copy ID into another app; send unicode emoji, own custom emoji,
  other-guild emoji as fake nitro (renders inline for us); gif search + send;
  open profile from mention; role rename + color change; create/reorder/delete
  a channel in a test guild; pin a channel and restart → order persists.
- **Safety**: destructive entries (delete role/channel/message) require the
  two-step confirm; verify a plain misclick cannot destroy anything.
