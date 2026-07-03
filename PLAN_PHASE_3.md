# tuicord — PLAN #3: Rich Message Content

> Companion to `PLAN.md` (library) and `PLAN_PHASE_2.md` (client v1).
> Scope: everything that makes a *message* render like Discord — media (gifs,
> stickers, videos, emojis), role colors, embeds (V1 static + V2 interactive),
> reactions, and Discord entity links. Interaction *menus/pickers* are PLAN #4.

## Context

The client renders text messages through `store → markup → ChatView`. Media
plumbing exists at the widget level (`widget.Image`: Kitty graphics + ANSI
half-block fallback, upload cache, z-order) but nothing connects a Discord
attachment/embed/sticker to it. `store.Message` carries only
`{Author, Content, Timestamp, Nonce, Pending, Failed}` — no author ID, no
attachments, no embeds, no reactions. `markup.Span` has no color channel, so
role-colored mentions and gradient names have nowhere to land.

## Existing pieces to reuse (do not rewrite)

- `widget.Image` — Kitty protocol with upload cache + `ANSI()` half-block
  fallback. All media rendering funnels through it.
- `markup.Parse` + `Resolver` — extend, don't fork. Code-span suppression and
  the single-pass parser stay as-is.
- `store.Role{Color, Position}` / `Member{Color, RoleIDs}` — role data is
  already synced (GUILD_ROLE_* handlers live in `internal/app`). Color math is
  the only missing piece.
- `app.Post` discipline — every fetch/decode happens on a goroutine, results
  land via `Post`. No exceptions for media.
- `input.MouseEvent` (`ButtonLeft`, press/release) — V2 embed buttons and link
  activation ride the existing hit-test path in `tui.App.handleMouse`.

## Library gaps to close first (small, additive — you implement)

1. **Per-span color in `screen.Style` flow** — `markup.Span` gains
   `FG uint32` (0 = inherit) and `chatview.mergeStyle` honors it. Gradient
   names need per-*cell* fg, so `chatSegment` must allow style changes
   mid-word (it already splits on style — verify no merge collapses them).
2. **Inline image slots in ChatView** — ChatView is a text viewport; images
   can't flow inline. Add a *block* concept: a rendered message is a list of
   blocks, each either text lines or an image region (w×h cells + `*Image`).
   ChatView reserves the cells, draws the image into the region on `Draw`.
3. **Clickable spans** — `chatLine` segments carry an optional `Action` (open
   link, jump to message, press button). ChatView maps `MousePress` x/y →
   segment → action. This is the foundation PLAN #4 builds its menus on.

## New packages / extensions

### `internal/store` — richer Message (pure, TDD)

```go
Message {
  ...existing,
  AuthorID   UserID          // for role color + profile lookups (PLAN #4)
  Attachments []Attachment   // {URL, ProxyURL, Filename, ContentType, W, H, Size}
  Embeds      []Embed        // normalized V1 embed (below)
  Stickers    []Sticker      // {ID, Name, Format}  Format: PNG/APNG/GIF/Lottie
  Reactions   []Reaction     // {EmojiName, EmojiID, Animated, Count, Me}
  Components  []Component    // V2: {Kind, Label, CustomID, Style, URL, Disabled}
}
Embed { Kind, Color, AuthorName, Title, URL, Description, Fields []{Name, Value, Inline},
        FooterText, ImageURL, ThumbURL, VideoURL, Provider } // Kind: rich|image|video|gifv|link
```

- New handlers in `internal/app`: `MESSAGE_UPDATE` (embeds arrive *after*
  MESSAGE_CREATE once Discord unfurls links — must patch in place by ID),
  `MESSAGE_REACTION_ADD/REMOVE/REMOVE_ALL`.
- Store methods: `UpdateMessage(id, patch)`, `AddReaction`, `RemoveReaction`.
- `convert.go` maps arikawa attachments/embeds/sticker_items/components.

### `internal/media` — fetch + decode + cache (new)

- `Fetch(url) → image.Image`: HTTP GET (proxy URL preferred — Discord CDN
  resizes via `?width=&height=&format=`), decode png/jpeg/gif/webp
  (add dep `golang.org/x/image/webp`), downscale to a cell budget.
- **Two-level cache**: in-memory LRU (decoded frames, ~64 entries) + disk
  (`~/.cache/tuicord/media/<sha256>`), so scrollback doesn't re-fetch.
- `FetchGIF(url) → []Frame{Image, Delay}` for animation.
- All calls async; a `Job` handle posts completion via `app.Post`. Placeholder
  glyph box (`▒▒ filename ▒▒`) while loading; error state on failure.
- Config: `[media] enabled=true, max_height_cells=12, animate=true`.
- Pure decode/downscale core → table tests; HTTP behind an interface.

### GIFs (tenor, giphy, klipy, plain links)

- A gif message is usually just a URL; Discord attaches an `Embed{Kind:gifv,
  VideoURL/ThumbURL}` on MESSAGE_UPDATE. Render rule: message whose content is
  *exactly* one URL and which has a gifv/image embed → suppress the raw URL,
  show the media block only (Discord parity).
- Source resolution: use the embed's proxy thumb/video first (works for tenor,
  giphy, klipy, anything Discord unfurls — no per-provider code). Direct
  `.gif` attachments/links decode natively.
- Animation: Kitty graphics animation (transmit frames, `a=a` control) when
  `media.animate`; else first frame + `[GIF]` badge. Animation ticker drives
  redraw via `Post`; pause when channel not active.

### Stickers

- Regular: `Sticker{ID, Format}` → `https://media.discordapp.net/stickers/<id>.png`
  (PNG/APNG; APNG = first frame v1), `.gif` for GIF format. **Lottie cannot be
  rendered** → fallback chip `[sticker: name]`.
- **Fake-nitro stickers**: content that is a bare sticker CDN link
  (`media.discordapp.net/stickers/<id>...` or `cdn.discordapp.com/stickers/...`)
  renders *as* a sticker block, URL suppressed. Detection lives in one pure
  func `media.ClassifyURL(url) → {Sticker|Emoji|GIF|Image|Video|Plain}` — TDD.

### Videos

- Attachment `ContentType video/*` or `Embed{Kind:video}` → render poster
  thumbnail (Discord provides `?format=jpeg` on the proxy URL for video
  attachments) + overlay line `▶ filename (1.2 MB, 0:42)`.
- No in-terminal playback. Clickable action "open" (via `Action` span) spawns
  `xdg-open`/configured player — config `[media] video_player = "mpv"`.

### Emojis

- Unicode emoji: already flow as text (width via `internal/tui/text`).
- Custom `<:name:id>` / `<a:name:id>`: markup already tokenizes to
  `Kind_Emoji`; extend the span to carry the ID. Render `:name:` styled; when
  Kitty available and `media.emoji_images=true`, render a 1-cell (2-col)
  inline image from `cdn.discordapp.com/emojis/<id>.png` (`.gif` if animated)
  through the inline-block mechanism — emoji-only messages render large
  (jumboable, Discord parity: ≤ 30 emojis and nothing else → big).
- **Fake-nitro emojis**: bare `cdn.discordapp.com/emojis/<id>` links (usually
  with `&name=` query) → classify + render as an emoji, not a link/image.

### Role colors (mentions, authors, "rich" gradients)

- **`<@&id>` role mentions**: new `Kind_RoleMention` + `Resolver.Role(id) →
  (name string, color uint32, ok bool)`. Renders `@RoleName` in the role color
  (bg tint like Discord is optional; fg color is v1).
- **Author names**: color = highest-`Position` role with `Color != 0` among
  `Member.RoleIDs` (Discord's rule). Pure func
  `store.MemberColor(guild, user) uint32` — TDD (no roles, colorless roles,
  position ties).
- **Rich role colors (nitro gradient/holographic)**: arikawa exposes
  `role.Colors{Primary, Secondary, Tertiary}`. Extend `store.Role` with
  `Colors [3]uint32`. Render: linear interpolation across the *cells* of the
  name (pure `lerpColor(a, b, t)` — TDD); holographic (tertiary set) uses the
  fixed Discord 3-stop palette. Degrades to primary color when the terminal
  lacks truecolor (existing screen degradation handles this).

### Embeds V1 (static)

- New `internal/ui/embedview.go`: renders one `store.Embed` into chat blocks:
  `▍` colored gutter (embed color) down the left, author line, bold title
  (clickable if URL), description through `markup.Parse`, fields as a 2-col
  grid (inline) or stacked, footer dim, thumbnail as a small right-side image
  block, large image as a bottom block.
- Pure layout core (`embed → []block` given width) — golden/table tests.

### Embeds V2 (Components — INTERACTIVE)

- Components arrive in `message.components` (action rows → buttons / selects;
  V2 adds sections/containers). v1 scope: **buttons + link buttons**; selects
  render disabled with a `[select: …]` chip.
- Render: buttons as `⟦ Label ⟧` chips on their own line, styled by button
  style (primary/danger/…), each carrying an `Action`.
- **Interaction**: clicking a non-link button sends
  `POST /interactions` (`type: 3` MESSAGE_COMPONENT, with `application_id`,
  `channel_id`, `message_id`, `session_id` (gateway session), `custom_id`,
  nonce) — arikawa user-account fork has the ground; add
  `internal/discord/interactions.go` if missing. Button shows a pending
  spinner until gateway ack/`INTERACTION_*` event or 3 s timeout → toast error
  (reuse `ui/toast.go`).
- This forces the **clickable spans** library gap — the button is a span
  action, not a separate widget tree inside the viewport (keeps ChatView's
  scroll model intact).

### Reactions

- Line under the message: `⤷ 👍 3 · :pepe: 12` — custom emoji by name, `Me`
  highlighted (inverse). Clicking a reaction chip toggles it
  (`PUT/DELETE /channels/<c>/messages/<m>/reactions/<emoji>/@me`) —
  optimistic count bump, reconcile on gateway event.

### Links to messages, channels, users, servers

- In `markup`: recognize `https://discord.com/channels/<g>/<c>[/<m>]` and
  `discord.gg/<invite>` → new `Kind_MessageLink` / `Kind_ChannelLink` /
  `Kind_InviteLink`, resolved to pills like `#general ↷ message` /
  `⌂ ServerName` via Resolver. `<#id>`, `<@id>` already handled; add `<@&id>`
  (above) and `<t:...>` timestamps while in there (cheap, same token shape).
- Click: message link scrolls/loads that message (`LoadHistory` around ID);
  channel link switches channel; user link is a PLAN #4 hook; invite link
  shows a toast with server name (`GET /invites/<code>`), join is PLAN #4.

## Build order

1. **Store + convert + handlers**: Message fields, MESSAGE_UPDATE, reactions
   events, `MemberColor`, `Role.Colors`. All pure — TDD first.
2. **Markup extensions**: Span color/ID/action fields, `Kind_RoleMention`,
   discord.com link kinds, `<t:>` timestamps. TDD.
3. **Role colors end-to-end**: author names + role mentions + gradients in
   ChatView (no media yet — pure color plumbing, easy to verify visually).
4. **`internal/media`**: fetch/cache/decode/classify. TDD the pure parts.
5. **Inline blocks in ChatView** (library gap 2) → attachments/images render.
6. **GIFs + stickers + emojis** (incl. fake-nitro classify), then videos.
7. **Embed V1 view**, then **reactions line**.
8. **Clickable spans** (gap 3) → links, video open, reaction toggle.
9. **Embeds V2 buttons + interactions endpoint** (riskiest, last).

## Definition of done

Every new/extended package keeps the house rules: package comment,
`example_test`, 80%+ on pure cores (`store` patch/reactions, `markup` new
kinds, `media` classify/downscale, embed layout, color lerp), `go vet` clean,
all width math via `internal/tui/text`. Media never blocks the UI thread;
every network result enters via `app.Post`.

## Verification

- **Unit**: `go test ./internal/store/... ./internal/markup/... ./internal/media/...` 80%+.
- **Live**: a test channel with — tenor gif, giphy gif, uploaded .gif, sticker,
  Lottie sticker (chip fallback), fake-nitro sticker link, fake-nitro emoji
  link, `<@&role>` mention, gradient-role author, V1 embed (bot), V2 buttons
  (bot), video attachment, reactions incl. own.
- **Degradation**: same channel under `NO_COLOR=1`, non-Kitty terminal
  (half-block fallback), `media.enabled=false` (chips only), 80×24.
- **Interactivity**: click V2 button on a known bot → bot responds; click
  reaction → count bumps then reconciles; click message link → jumps.
