# tuicord — PLAN #5: Forums, Threads & Special Channel Kinds

> Companion to `PLAN_PHASE_3.md` (rich content) and `PLAN_PHASE_4.md`
> (interaction layer). Scope: the channel types the client currently can't
> represent — **threads**, **forum channels**, **announcement (news)
> channels**, and **rules channels** — plus the navigation model they force
> (a channel that contains *channels*, not messages).

## Context

`store.ChannelKind` knows only `Text | Voice | Category | DM`. Discord's
type field is richer: 5 = GUILD_ANNOUNCEMENT, 10/11/12 = announcement/
public/private threads, 15 = GUILD_FORUM (16 = GUILD_MEDIA behaves the same).
"Rules" is not a type — it's a plain text channel referenced by
`guild.rules_channel_id` — but it deserves its own badge and read-only
affordance. Threads break the current 1-level `guild → channel → messages`
model: they are channels *parented to* a channel, arrive via dedicated
gateway events, and are lazily listed (active vs archived).

## Existing pieces to reuse

- `store` channel machinery — threads are channels with a `ParentID`; the
  ring-buffer message store works unchanged per-thread.
- PLAN #4 `widget.Menu` / `Tabs` / category grouping in the channel list —
  thread sub-items and forum post lists reuse the same indent/collapse UI.
- PLAN #3 embeds/media — forum posts are just messages; the forum *list* view
  reuses `ItemList` with badge/preview styling.
- `app.LoadHistory` / `beginHistoryLoad` gating pattern — copy for thread
  list fetches and archived-thread pagination.

## Store & data model

- **Extend `ChannelKind`**: `ChannelAnnouncement`, `ChannelForum`,
  `ChannelThread` (one kind; the public/private/announcement distinction is a
  `ThreadMeta` field, they render the same).
- **`Channel` gains**: `ParentID ChannelID`, `Thread *ThreadMeta` where
  `ThreadMeta{Archived, Locked, MessageCount, MemberCount, OwnerID,
  LastActive time.Time, Joined bool}`.
- **Forum extras**: `Channel.Forum *ForumMeta{Tags []Tag{ID, Name, Emoji},
  DefaultSort}`; a forum *post* is a thread whose `AppliedTags []TagID` is set.
- **Guild gains**: `RulesChannelID ChannelID` (from GUILD_CREATE).
- Store methods: `Threads(parent ChannelID) []Channel` (active, sorted by
  `LastActive` desc), `UpsertThread`, `RemoveThread`, `SetArchived`.
  All pure — TDD (archive transitions, sort, parent reassignment guards).

## Gateway & REST (`internal/app`, `convert.go`)

- Map arikawa channel types 5/10/11/12/15/16 in `convert.go` (unknown types
  degrade to `ChannelText` — never drop a channel on the floor).
- New handlers: `THREAD_CREATE`, `THREAD_UPDATE`, `THREAD_DELETE`,
  `THREAD_LIST_SYNC` (bulk active threads on guild subscribe),
  `THREAD_MEMBER_UPDATE` (own join/leave → `Joined`).
- REST loads (same begin/finish gating as history):
  - `GET /guilds/<g>/threads/active` on guild open.
  - `GET /channels/<c>/threads/archived/public` — paginated (`before`
    timestamp), fetched on demand ("Load archived…" list entry).
  - `POST /channels/<c>/threads` (create in text/announcement channel, with
    `message_id` for message-anchored threads) and forum-post create
    (`POST /channels/<forum>/threads` with embedded first message + tags).
  - `PUT/DELETE /channels/<t>/thread-members/@me` (join/leave).

## UI

### Channel sidebar

- Kind badges before the name: `#` text, `📣`/`!` announcement, `☰` forum,
  `⤷` thread, and `§` on the rules channel (`RulesChannelID` match).
  Pure `channelBadge(Channel, Guild) rune` — TDD. (All glyphs width-checked
  through `internal/tui/text`; ASCII fallbacks under `NO_COLOR`/no-unicode
  config.)
- Active threads nest under their parent, indented one level, collapsible
  like PLAN #4 categories. Threads with unread use the existing badge path.

### Threads (in text/announcement channels)

- Open thread = it becomes the active "channel" — ChatView works unchanged.
  Breadcrumb in the chat header: `#parent ⤷ thread-name`; Esc/`h` goes back
  to the parent.
- PLAN #4 message context menu gains **"Create thread…"** (name prompt →
  message-anchored thread) and thread list entries gain **Join/Leave**,
  **Archive/Unarchive** (needs MANAGE_THREADS or ownership — reuse PLAN #4
  `HasPermission`).
- A message that *started* a thread shows a `⤷ thread-name (N messages)`
  action line (clickable span → opens the thread).

### Forum channels

- Selecting a forum shows a **post list view** instead of ChatView: one row
  per thread — title, tag chips (colored), reply count, last-active, unread
  badge. `ItemList`; sort per `DefaultSort` (latest activity default);
  a tag-filter row on top (click tag chip → filter, PLAN #4 Menu for the
  full tag list). "Load archived…" footer row paginates.
- Enter/click a post → opens it as a thread (same ChatView path). Composer in
  the post list view creates a **new post**: Modal with title `TextInput`,
  tag picker (`ItemList` multi-select), body composer.
- New `internal/ui/forumview.go`; pure row-building core (`post → row`
  label/badges given width) — TDD.

### Announcement & rules channels

- Announcement channels are text channels + badge; if we lack SEND_MESSAGES
  (typical), composer renders disabled with a `read-only` hint — this is a
  general permission feature (compute per-channel send permission incl.
  overwrites: extend PLAN #4 permission calc with `PermissionOverwrites` on
  `Channel` — pure, TDD; rules channels get the same treatment for free).
- PLAN #4 message menu gains **"Publish"** (`POST …/messages/<id>/crosspost`)
  on own messages in announcement channels.

## Build order

1. **Store/convert**: kinds, `ParentID`, `ThreadMeta`/`ForumMeta`,
   `RulesChannelID`, thread methods — TDD.
2. **Gateway handlers + active-thread load** — threads appear in sidebar,
   read-only open/back navigation. (Core value; ship before anything else.)
3. **Channel badges + permission-aware composer** (announcement/rules
   read-only, overwrites calc).
4. **Thread actions**: create from message, join/leave, archive, archived
   pagination.
5. **Forum post list view** + open post, then tag filter, then post creation.
6. **Publish (crosspost)** menu entry.

## Definition of done

House rules: package comments, example_tests, 80%+ pure cores (thread
sort/archive transitions, badge selection, overwrite permission calc, forum
row building), `go vet` clean, unknown channel types never crash or vanish,
all fetches gated + posted via `app.Post`, 80×24 + `NO_COLOR` clean.

## Verification

- **Unit**: type mapping incl. unknown types, thread lifecycle
  (create→archive→unarchive→delete), overwrite calc (deny SEND on @everyone,
  allow on role, member-level deny), forum sort/tag filter.
- **Live**: guild with forum + announcement + rules + active threads —
  threads nest and open; create a thread from a message; post in a forum with
  tags; archived posts paginate; composer disabled in rules channel;
  publish a message in an announcement channel; leave/join a thread and see
  `Joined` reflect via THREAD_MEMBER_UPDATE.
