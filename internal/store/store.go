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
	ID       ChannelID
	GuildID  GuildID
	Name     string
	Kind     ChannelKind
	Position int
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
}

// New returns an empty store. A historyLimit <= 0 uses DefaultHistoryLimit.
func New(historyLimit int) *Store {
	if historyLimit <= 0 {
		historyLimit = DefaultHistoryLimit
	}
	return &Store{
		historyLimit: historyLimit,
		guilds:       map[GuildID]Guild{},
		channelOrder: map[GuildID][]ChannelID{},
		channels:     map[ChannelID]Channel{},
		messages:     map[ChannelID]*ring{},
		members:      map[GuildID]map[UserID]Member{},
		roles:        map[GuildID]map[RoleID]Role{},
		unread:       map[ChannelID]int{},
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
