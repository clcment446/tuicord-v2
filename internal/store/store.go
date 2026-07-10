// Package store holds the client's normalized view of Discord state: guilds,
// channels, messages, and members.
//
// The store is deliberately free of any Discord library types so it can be
// unit-tested in isolation and so the rendering code depends only on plain
// data. The orchestration layer (internal/app) translates gateway events into
// store mutations.
//
// # Rich message content
//
// [Message] carries the full set of Discord rich content: [Attachment] slices,
// [Embed] slices (which arrive asynchronously via MESSAGE_UPDATE once Discord
// unfurls links), [Sticker] slices, [Reaction] slices, legacy [Component]
// slices, and a hierarchical [ComponentNode] tree for Components V2. Use
// [Store.UpdateMessage],
// [Store.AddReaction], and [Store.RemoveReaction] to patch messages in place
// after the initial append.
//
// # Role colors
//
// [Store.MemberColor] returns a member's effective display color following
// Discord's rule: the highest-position role with a non-zero color wins.
// [Role] carries a [Role.Colors] gradient triple for nitro gradient roles;
// [LerpColor] and [Role.GradientAt] provide the interpolation math.
//
// # Concurrency
//
// The store is not safe for concurrent use. By convention it is mutated and
// read only on the UI goroutine: gateway handlers marshal their changes through
// tui.App.Post, which runs them on that goroutine, and widgets read the store
// during Draw on the same goroutine. Keeping to that rule means no locks are
// needed.
package store

import "time"

// ID types mirror Discord snowflakes as opaque 64-bit identifiers.
type (
	// GuildID identifies a guild (server).
	GuildID uint64
	// ChannelID identifies a channel.
	ChannelID uint64
	// MessageID identifies a message.
	MessageID uint64
	// UserID identifies a user.
	UserID uint64
	// RoleID identifies a role within a guild.
	RoleID uint64
)

// ChannelKind distinguishes the channel types the client renders.
type ChannelKind int

const (
	// ChannelText is a standard text channel.
	ChannelText ChannelKind = iota
	// ChannelVoice is a voice channel (listed but not joinable in v1).
	ChannelVoice
	// ChannelCategory groups channels in the sidebar.
	ChannelCategory
	// ChannelDM is a direct-message channel.
	ChannelDM
)

// Guild is a Discord server.
type Guild struct {
	ID   GuildID
	Name string
	// OwnerID is the guild owner's user ID. The owner implicitly holds every
	// permission (see Store.MemberPermissions). Zero when unknown.
	OwnerID UserID
}

// Channel is a channel within a guild.
type Channel struct {
	ID      ChannelID
	GuildID GuildID
	Name    string
	Kind    ChannelKind
	// Position is the channel's sort key within the guild (or within its
	// category). ParentID is the category channel this one is nested under, or
	// zero for top-level channels; it drives category grouping in the sidebar.
	Position int
	ParentID ChannelID
}

// GuildFolder groups guilds in the sidebar rail, mirroring Discord's
// user_settings.guild_folders. Discord represents an un-foldered guild as a
// single-element folder with an empty Name; [OrderGuilds] renders those as bare
// top-level guilds rather than folders.
type GuildFolder struct {
	// ID is the folder identifier. It is zero for the synthetic single-guild
	// folders Discord uses to place un-foldered guilds.
	ID   int64
	Name string
	// Color is the folder's 0xRRGGBB tint, or zero when unset.
	Color    uint32
	GuildIDs []GuildID
}

// Message is a single chat message. Pending marks an optimistic local message
// awaiting the gateway echo; Failed marks one whose REST send returned an error.
//
// The rich-content fields (Attachments, Embeds, Stickers, Reactions,
// Components, ComponentTree) may be empty initially and patched later via UpdateMessage,
// AddReaction, and RemoveReaction once the gateway delivers the full data.
type Message struct {
	ID        MessageID
	ChannelID ChannelID
	// AuthorID is the snowflake of the sending user, used for role-color and
	// profile lookups.
	AuthorID UserID
	// ApplicationID is set on messages sent by an application (interaction
	// responses, webhooks). Component interactions are addressed to it; when
	// zero, the bot author's snowflake doubles as the application ID.
	ApplicationID uint64
	Author        string
	Content       string
	Timestamp     time.Time
	Nonce         string
	Flags         uint64
	Pending       bool
	Failed        bool
	Attachments   []Attachment
	Embeds        []Embed
	Stickers      []Sticker
	Reactions     []Reaction
	Components    []Component
	// ComponentTree preserves Discord's hierarchical Components V2 layout.
	ComponentTree []ComponentNode
	Pinned        bool
}

// Member is a guild member, used to resolve mentions.
type Member struct {
	ID      UserID
	Name    string
	Color   uint32
	RoleIDs []RoleID
}

// Role is a Discord role used to interpret member role IDs.
//
// Nitro gradient roles carry up to three color stops in Colors. When Colors is
// all zero the role uses the flat Color value. Use GradientAt to obtain the
// interpolated color for a given position along the name.
type Role struct {
	ID          RoleID
	Name        string
	Position    int
	Color       uint32
	Hoist       bool
	Mentionable bool
	// Colors holds the gradient color stops [Primary, Secondary, Tertiary].
	// A zero entry means the stop is unset. Only Primary non-zero → solid
	// color; Primary+Secondary → two-stop linear; all three → holographic
	// three-stop interpolation.
	Colors [3]uint32
	// Permissions is the role's guild-level permission bit set.
	Permissions Permission
}

// GuildEmoji is a custom emoji in a guild's catalog, used to populate the emoji
// picker. Unicode emoji are not stored here; they come from the picker's static
// table.
type GuildEmoji struct {
	ID       uint64
	Name     string
	Animated bool
}

// GuildSticker is a custom sticker in a guild's catalog, used to populate the
// sticker picker.
type GuildSticker struct {
	ID     uint64
	Name   string
	Format StickerFormat
}

// DefaultHistoryLimit is the per-channel message ring size when none is given.
const DefaultHistoryLimit = 200

// Store is the normalized client state. Construct it with New.
type Store struct {
	historyLimit int

	guildOrder []GuildID
	guilds     map[GuildID]Guild

	channelOrder map[GuildID][]ChannelID
	channels     map[ChannelID]Channel

	messages map[ChannelID]*ring

	members map[GuildID]map[UserID]Member
	roles   map[GuildID]map[RoleID]Role

	unread map[ChannelID]int

	guildFolders  []GuildFolder
	guildEmojis   map[GuildID][]GuildEmoji
	guildStickers map[GuildID][]GuildSticker
	// hasNitro reflects the logged-in account's Nitro status, which decides
	// whether custom emoji can be sent inline natively or must fall back to the
	// fake-nitro CDN URL.
	hasNitro bool
}

// New returns an empty store. A historyLimit <= 0 uses DefaultHistoryLimit.
func New(historyLimit int) *Store {
	if historyLimit <= 0 {
		historyLimit = DefaultHistoryLimit
	}
	return &Store{
		historyLimit:  historyLimit,
		guilds:        map[GuildID]Guild{},
		channelOrder:  map[GuildID][]ChannelID{},
		channels:      map[ChannelID]Channel{},
		messages:      map[ChannelID]*ring{},
		members:       map[GuildID]map[UserID]Member{},
		roles:         map[GuildID]map[RoleID]Role{},
		unread:        map[ChannelID]int{},
		guildEmojis:   map[GuildID][]GuildEmoji{},
		guildStickers: map[GuildID][]GuildSticker{},
	}
}

// IncrementUnread bumps a channel's unread counter.
func (s *Store) IncrementUnread(channel ChannelID) {
	s.unread[channel]++
}

// ClearUnread resets a channel's unread counter to zero.
func (s *Store) ClearUnread(channel ChannelID) {
	delete(s.unread, channel)
}

// Unread returns a channel's unread message count.
func (s *Store) Unread(channel ChannelID) int {
	return s.unread[channel]
}

// UpsertGuild inserts or updates a guild, preserving first-seen order.
func (s *Store) UpsertGuild(g Guild) {
	if existing, ok := s.guilds[g.ID]; ok {
		if g.Name == "" {
			g.Name = existing.Name
		}
		if g.OwnerID == 0 {
			g.OwnerID = existing.OwnerID
		}
	}
	if _, ok := s.guilds[g.ID]; !ok {
		s.guildOrder = append(s.guildOrder, g.ID)
	}
	s.guilds[g.ID] = g
}

// SetGuildFolders records the guild-folder layout from READY (or a later
// USER_SETTINGS update). Passing nil clears it, in which case the sidebar falls
// back to first-seen guild order.
func (s *Store) SetGuildFolders(folders []GuildFolder) {
	s.guildFolders = append(s.guildFolders[:0], folders...)
}

// GuildFolders returns the current guild-folder layout, or nil when none is
// known.
func (s *Store) GuildFolders() []GuildFolder {
	if len(s.guildFolders) == 0 {
		return nil
	}
	out := make([]GuildFolder, len(s.guildFolders))
	copy(out, s.guildFolders)
	return out
}

// SetGuildEmojis replaces a guild's custom emoji catalog. Passing an empty
// slice clears it.
func (s *Store) SetGuildEmojis(guild GuildID, emojis []GuildEmoji) {
	if len(emojis) == 0 {
		delete(s.guildEmojis, guild)
		return
	}
	s.guildEmojis[guild] = append([]GuildEmoji(nil), emojis...)
}

// GuildEmojis returns a guild's custom emoji catalog in stored order.
func (s *Store) GuildEmojis(guild GuildID) []GuildEmoji {
	src := s.guildEmojis[guild]
	if len(src) == 0 {
		return nil
	}
	out := make([]GuildEmoji, len(src))
	copy(out, src)
	return out
}

// SetGuildStickers replaces a guild's sticker catalog. Passing an empty slice
// clears it.
func (s *Store) SetGuildStickers(guild GuildID, stickers []GuildSticker) {
	if len(stickers) == 0 {
		delete(s.guildStickers, guild)
		return
	}
	s.guildStickers[guild] = append([]GuildSticker(nil), stickers...)
}

// GuildStickers returns a guild's sticker catalog in stored order.
func (s *Store) GuildStickers(guild GuildID) []GuildSticker {
	src := s.guildStickers[guild]
	if len(src) == 0 {
		return nil
	}
	out := make([]GuildSticker, len(src))
	copy(out, src)
	return out
}

// SetNitro records whether the logged-in account has Discord Nitro.
func (s *Store) SetNitro(v bool) { s.hasNitro = v }

// HasNitro reports whether the logged-in account has Discord Nitro.
func (s *Store) HasNitro() bool { return s.hasNitro }

// Guilds returns guilds in first-seen order.
func (s *Store) Guilds() []Guild {
	out := make([]Guild, 0, len(s.guildOrder))
	for _, id := range s.guildOrder {
		out = append(out, s.guilds[id])
	}
	return out
}

// GuildName resolves a guild's display name, returning ok=false when unknown.
func (s *Store) GuildName(id GuildID) (string, bool) {
	g, ok := s.guilds[id]
	return g.Name, ok
}

// UpsertChannel inserts or updates a channel. Channels are returned from
// Channels sorted by Position then ID, so insertion order does not matter.
func (s *Store) UpsertChannel(c Channel) {
	if _, ok := s.channels[c.ID]; !ok {
		s.channelOrder[c.GuildID] = append(s.channelOrder[c.GuildID], c.ID)
	}
	s.channels[c.ID] = c
}

// Channel returns the channel with id, if known.
func (s *Store) Channel(id ChannelID) (Channel, bool) {
	c, ok := s.channels[id]
	return c, ok
}

// Channels returns a guild's channels sorted by Position, then ID.
func (s *Store) Channels(guild GuildID) []Channel {
	ids := s.channelOrder[guild]
	out := make([]Channel, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.channels[id])
	}
	sortChannels(out)
	return out
}

// AppendMessage adds a message to its channel's ring, evicting the oldest when
// the history limit is exceeded.
func (s *Store) AppendMessage(m Message) {
	r := s.messages[m.ChannelID]
	if r == nil {
		r = newRing(s.historyLimit)
		s.messages[m.ChannelID] = r
	}
	r.push(m)
}

// SetMessages replaces a channel's history with messages in oldest-first order.
func (s *Store) SetMessages(channel ChannelID, messages []Message) {
	if len(messages) == 0 {
		delete(s.messages, channel)
		return
	}
	r := newRing(s.historyLimit)
	for _, m := range messages {
		m.ChannelID = channel
		r.push(m)
	}
	s.messages[channel] = r
}

// ReplaceMessage swaps a pending optimistic message (matched by Nonce) for the
// confirmed message from the gateway. It reports whether a match was found; if
// not, the caller should AppendMessage instead.
func (s *Store) ReplaceMessage(nonce string, confirmed Message) bool {
	if nonce == "" {
		return false
	}
	r := s.messages[confirmed.ChannelID]
	if r == nil {
		return false
	}
	return r.replaceByNonce(nonce, confirmed)
}

// MarkFailed flags the pending message with the given nonce as failed to send.
func (s *Store) MarkFailed(channel ChannelID, nonce string) bool {
	r := s.messages[channel]
	if r == nil {
		return false
	}
	return r.markFailed(nonce)
}

// Messages returns a channel's messages oldest-first.
func (s *Store) Messages(channel ChannelID) []Message {
	r := s.messages[channel]
	if r == nil {
		return nil
	}
	return r.slice()
}

// UpsertMember inserts or updates a guild member.
func (s *Store) UpsertMember(guild GuildID, m Member) {
	byUser := s.members[guild]
	if byUser == nil {
		byUser = map[UserID]Member{}
		s.members[guild] = byUser
	}
	byUser[m.ID] = m
}

// RemoveMember deletes a guild member.
func (s *Store) RemoveMember(guild GuildID, user UserID) {
	delete(s.members[guild], user)
}

// Members returns a guild's members in no particular order.
func (s *Store) Members(guild GuildID) []Member {
	byUser := s.members[guild]
	out := make([]Member, 0, len(byUser))
	for _, m := range byUser {
		out = append(out, m)
	}
	return out
}

// Member returns a guild member by ID.
func (s *Store) Member(guild GuildID, user UserID) (Member, bool) {
	m, ok := s.members[guild][user]
	return m, ok
}

// MemberName resolves a user's display name within a guild, returning ok=false
// when the member is unknown.
func (s *Store) MemberName(guild GuildID, user UserID) (string, bool) {
	if m, ok := s.members[guild][user]; ok {
		return m.Name, true
	}
	return "", false
}

// UpsertRole inserts or updates a guild role.
func (s *Store) UpsertRole(guild GuildID, r Role) {
	byRole := s.roles[guild]
	if byRole == nil {
		byRole = map[RoleID]Role{}
		s.roles[guild] = byRole
	}
	byRole[r.ID] = r
}

// RemoveRole deletes a guild role.
func (s *Store) RemoveRole(guild GuildID, role RoleID) {
	delete(s.roles[guild], role)
}

// Role resolves a guild role by ID.
func (s *Store) Role(guild GuildID, role RoleID) (Role, bool) {
	r, ok := s.roles[guild][role]
	return r, ok
}

// Roles returns all cached roles for a guild.
func (s *Store) Roles(guild GuildID) []Role {
	byRole := s.roles[guild]
	out := make([]Role, 0, len(byRole))
	for _, r := range byRole {
		out = append(out, r)
	}
	sortRoles(out)
	return out
}

// ChannelName resolves a channel's name, returning ok=false when unknown.
func (s *Store) ChannelName(id ChannelID) (string, bool) {
	if c, ok := s.channels[id]; ok {
		return c.Name, true
	}
	return "", false
}

func sortChannels(cs []Channel) {
	// Insertion sort keeps it dependency-free and is fine for sidebar sizes.
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && lessChannel(cs[j], cs[j-1]); j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}

func lessChannel(a, b Channel) bool {
	if a.Position != b.Position {
		return a.Position < b.Position
	}
	return a.ID < b.ID
}

func sortRoles(rs []Role) {
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && lessRole(rs[j], rs[j-1]); j-- {
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
}

func lessRole(a, b Role) bool {
	if a.Position != b.Position {
		return a.Position > b.Position
	}
	return a.ID < b.ID
}
