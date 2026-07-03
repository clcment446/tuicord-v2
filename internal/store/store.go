// Package store holds the client's normalized view of Discord state: guilds,
// channels, messages, and members.
//
// The store is deliberately free of any Discord library types so it can be
// unit-tested in isolation and so the rendering code depends only on plain
// data. The orchestration layer (internal/app) translates gateway events into
// store mutations.
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
type Message struct {
	ID        MessageID
	ChannelID ChannelID
	Author    string
	Content   string
	Timestamp time.Time
	Nonce     string
	Pending   bool
	Failed    bool
}

// Member is a guild member, used to resolve mentions.
type Member struct {
	ID    UserID
	Name  string
	Color uint32
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

// Members returns a guild's members in no particular order.
func (s *Store) Members(guild GuildID) []Member {
	byUser := s.members[guild]
	out := make([]Member, 0, len(byUser))
	for _, m := range byUser {
		out = append(out, m)
	}
	return out
}

// MemberName resolves a user's display name within a guild, returning ok=false
// when the member is unknown.
func (s *Store) MemberName(guild GuildID, user UserID) (string, bool) {
	if m, ok := s.members[guild][user]; ok {
		return m.Name, true
	}
	return "", false
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
