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
	// ChannelAnnouncement is a news channel: a text channel whose messages can
	// be published (crossposted) to following servers.
	ChannelAnnouncement
	// ChannelForum is a channel that contains only threads (posts) rather than
	// messages. Selecting one shows a post list instead of a chat view.
	ChannelForum
	// ChannelThread is a sub-channel parented to a text, announcement, or forum
	// channel. The public/private/announcement distinction is carried in
	// [ThreadMeta]; they all render the same. Forum posts are threads too.
	ChannelThread
)

// Guild is a Discord server.
type Guild struct {
	ID   GuildID
	Name string
	// OwnerID is the guild owner's user ID. The owner implicitly holds every
	// permission (see Store.MemberPermissions). Zero when unknown.
	OwnerID UserID
	// RulesChannelID is the guild's designated rules channel, if any. It is a
	// plain text channel (not a distinct type) that the sidebar decorates with a
	// rules badge and renders read-only. Zero when unset.
	RulesChannelID ChannelID
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
	// For threads, ParentID is the text/announcement/forum channel they hang off.
	Position int
	ParentID ChannelID
	// RecipientIDs identifies the users in a DM/group-DM. It is empty for
	// guild channels and lets profile actions find existing shared DMs without
	// relying on mutable display names.
	RecipientIDs []UserID
	// Overwrites are the channel's permission overrides for roles and members,
	// applied on top of guild-level permissions by [Store.ChannelPermissions].
	Overwrites []PermissionOverwrite
	// Thread is non-nil when Kind is [ChannelThread], carrying thread-specific
	// metadata (archive/lock state, counts, membership).
	Thread *ThreadMeta
	// Recipients contains the users participating in a direct or group DM.
	// Discord may omit it from later sparse channel payloads, so ingestion must
	// preserve an already hydrated value in that case.
	Recipients []Member
	// Forum is non-nil when Kind is [ChannelForum], carrying the available tag
	// set and default sort order. A forum post is a [ChannelThread] whose
	// ThreadMeta.AppliedTags references these tags.
	Forum *ForumMeta
}

// PermissionOverwrite is a single channel permission override, mirroring
// Discord's overwrite object: a role or member and the permission bits it
// allows and denies. [Store.ChannelPermissions] folds these over the member's
// guild-level permissions.
type PermissionOverwrite struct {
	// ID is the role or member snowflake the overwrite targets.
	ID uint64
	// Role is true when the overwrite targets a role, false when a member.
	Role  bool
	Allow Permission
	Deny  Permission
}

// ThreadSort selects how a forum's posts are ordered in the post list.
type ThreadSort int

const (
	// SortLatestActivity orders posts by most recent activity (the default).
	SortLatestActivity ThreadSort = iota
	// SortCreationDate orders posts newest-created first.
	SortCreationDate
)

// ThreadMeta carries the thread-specific fields a [ChannelThread] needs beyond
// the common [Channel] shape.
type ThreadMeta struct {
	Archived     bool
	Locked       bool
	MessageCount int
	MemberCount  int
	OwnerID      UserID
	// LastActive is the thread's most recent activity time, used to sort active
	// threads (descending) in the sidebar and forum post list.
	LastActive time.Time
	// Joined reports whether the logged-in account is a member of the thread,
	// tracked through THREAD_MEMBER_UPDATE.
	Joined bool
	// AppliedTags are the forum tag IDs applied to this post (empty for
	// non-forum threads).
	AppliedTags []uint64
}

// ForumMeta carries the forum-channel configuration a [ChannelForum] needs:
// the tags posts may carry and the default post ordering.
type ForumMeta struct {
	Tags        []Tag
	DefaultSort ThreadSort
}

// Tag is a forum tag that can be applied to posts. Emoji is the unicode glyph
// or empty when the tag has no (or a custom) emoji.
type Tag struct {
	ID    uint64
	Name  string
	Emoji string
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
	// AuthorAvatarURL is the Discord CDN URL for the author's profile picture.
	// It is populated from the message author so chat rendering does not depend
	// on a guild-member cache (which is absent for DMs and webhooks).
	AuthorAvatarURL string
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

	// rev is the store revision this version of the message was stamped with.
	// It is unexported so only the store can issue one, and it is a scalar so
	// that the shallow copies Messages returns carry it by value. That makes it
	// a true snapshot: the slice fields above alias the store's backing arrays
	// and can be patched in place underneath a copy, so comparing Message
	// values cannot detect a change — comparing revisions can.
	rev uint64
}

// Rev reports the store revision stamped on this message. It changes on every
// mutation the store makes, including in-place patches to the message's slices
// that a value comparison would miss. Renderers should treat a changed Rev as
// invalidating anything cached from this message.
func (m Message) Rev() uint64 { return m.rev }

// Member is a guild member, used to resolve mentions.
type Member struct {
	ID UserID
	// Name is the server-local display name used by mention rendering.
	Name     string
	Username string
	Nick     string
	// AvatarURL prefers the guild-specific profile picture when Discord provides
	// one, falling back to the user's global avatar.
	AvatarURL string
	Color     uint32
	RoleIDs   []RoleID
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
	// pings counts unread messages that require attention (a direct message or
	// a mention). It stays separate from unread so ordinary traffic never
	// reorders the sidebar or shows a red notification badge.
	pings map[ChannelID]int

	guildFolders  []GuildFolder
	guildEmojis   map[GuildID][]GuildEmoji
	guildStickers map[GuildID][]GuildSticker
	// hasNitro reflects the logged-in account's Nitro status, which decides
	// whether custom emoji can be sent inline natively or must fall back to the
	// fake-nitro CDN URL.
	hasNitro bool

	// rev is a monotonic counter stamped onto every message the store mutates.
	// It is never reset and never reused, so a revision identifies one version
	// of one message for the life of the store.
	rev uint64
	// metaRev counts mutations to everything a message render reads but does
	// not own: members, roles, channels, and guilds. Mention resolution depends
	// on those, so a render cached before members arrived must be discarded
	// even though the message itself never changed.
	metaRev uint64
}

// nextRevision returns a fresh, never-reused message revision.
func (s *Store) nextRevision() uint64 {
	s.rev++
	return s.rev
}

// MetaRev reports the current revision of non-message state (members, roles,
// channels, guilds). Renderers that resolve mentions or channel references
// should treat a change here as invalidating cached output.
func (s *Store) MetaRev() uint64 {
	return s.metaRev
}

// touchMeta records a mutation to non-message state.
func (s *Store) touchMeta() {
	s.metaRev++
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
		pings:         map[ChannelID]int{},
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
	delete(s.pings, channel)
}

// Unread returns a channel's unread message count.
func (s *Store) Unread(channel ChannelID) int {
	return s.unread[channel]
}

// IncrementPing bumps a channel's attention count. Callers must only use this
// for an inbound direct message or a message that actually mentions the user.
func (s *Store) IncrementPing(channel ChannelID) { s.pings[channel]++ }

// Pings returns the unread attention count for one channel.
func (s *Store) Pings(channel ChannelID) int { return s.pings[channel] }

// GuildPings returns the total attention count across a server's channels.
func (s *Store) GuildPings(guild GuildID) int {
	total := 0
	for _, channel := range s.channelOrder[guild] {
		total += s.pings[channel]
	}
	return total
}

// PingedChannels returns the channels with at least one attention message.
// The returned map is a snapshot suitable for sidebar ordering.
func (s *Store) PingedChannels() map[ChannelID]bool {
	out := make(map[ChannelID]bool, len(s.pings))
	for id, count := range s.pings {
		if count > 0 {
			out[id] = true
		}
	}
	return out
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
	s.touchMeta()
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

// Guild returns the guild with id, if known.
func (s *Store) Guild(id GuildID) (Guild, bool) {
	g, ok := s.guilds[id]
	return g, ok
}

// UpsertChannel inserts or updates a channel. Channels are returned from
// Channels sorted by Position then ID, so insertion order does not matter.
func (s *Store) UpsertChannel(c Channel) {
	if _, ok := s.channels[c.ID]; !ok {
		s.channelOrder[c.GuildID] = append(s.channelOrder[c.GuildID], c.ID)
	}
	s.channels[c.ID] = c
	s.touchMeta()
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
	m.rev = s.nextRevision()
	r.push(m)
}

// SetMessages replaces a channel's history with messages in oldest-first order.
//
// Local echoes still in flight are carried over and re-appended as the newest
// entries. A history load reflects the server's view from before the send, so
// replacing the ring wholesale would silently discard a message the user has
// just sent and is watching for — it would reappear only when the gateway echo
// arrived, or not at all.
func (s *Store) SetMessages(channel ChannelID, messages []Message) {
	echoes := s.unconfirmedEchoes(channel, messages)
	if len(messages) == 0 && len(echoes) == 0 {
		delete(s.messages, channel)
		return
	}
	r := newRing(s.historyLimit)
	for _, m := range messages {
		m.ChannelID = channel
		m.rev = s.nextRevision()
		r.push(m)
	}
	// Echoes keep their existing revision: they have not changed, so anything
	// caching a render of them stays valid.
	for _, m := range echoes {
		r.push(m)
	}
	s.messages[channel] = r
}

// unconfirmedEchoes returns the channel's pending and failed local echoes that
// incoming does not already account for. A nonce present in incoming means the
// server has confirmed that echo, so the incoming copy supersedes it.
func (s *Store) unconfirmedEchoes(channel ChannelID, incoming []Message) []Message {
	r := s.messages[channel]
	if r == nil {
		return nil
	}
	var confirmed map[string]struct{}
	for _, m := range incoming {
		if m.Nonce == "" {
			continue
		}
		if confirmed == nil {
			confirmed = make(map[string]struct{}, len(incoming))
		}
		confirmed[m.Nonce] = struct{}{}
	}
	var out []Message
	for _, m := range r.slice() {
		if !m.Pending && !m.Failed {
			continue
		}
		if _, ok := confirmed[m.Nonce]; ok && m.Nonce != "" {
			continue
		}
		out = append(out, m)
	}
	return out
}

// PrependMessages adds older messages before the existing channel history.
// Both slices must be ordered oldest-first.
func (s *Store) PrependMessages(channel ChannelID, messages []Message) {
	if len(messages) == 0 {
		return
	}
	current := s.Messages(channel)
	known := make(map[MessageID]struct{}, len(current))
	for _, message := range current {
		if message.ID != 0 {
			known[message.ID] = struct{}{}
		}
	}
	combined := make([]Message, 0, len(messages)+len(current))
	for _, message := range messages {
		if message.ID != 0 {
			if _, exists := known[message.ID]; exists {
				continue
			}
			known[message.ID] = struct{}{}
		}
		combined = append(combined, message)
	}
	combined = append(combined, current...)
	s.SetMessages(channel, combined)
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
	confirmed.rev = s.nextRevision()
	return r.replaceByNonce(nonce, confirmed)
}

// MarkFailed flags the pending message with the given nonce as failed to send.
func (s *Store) MarkFailed(channel ChannelID, nonce string) bool {
	r := s.messages[channel]
	if r == nil {
		return false
	}
	return r.markFailed(nonce, s.nextRevision())
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
	s.touchMeta()
}

// RememberMemberIdentity records a message author's identity without replacing
// guild-specific fields that only member/role gateway payloads can provide.
func (s *Store) RememberMemberIdentity(guild GuildID, m Member) {
	if guild == 0 || m.ID == 0 {
		return
	}
	byUser := s.members[guild]
	if byUser == nil {
		byUser = map[UserID]Member{}
		s.members[guild] = byUser
	}
	if existing, ok := byUser[m.ID]; ok {
		// Sparse message authors carry global identity only. Never replace a
		// guild nickname or guild avatar learned from a member payload.
		if existing.Name == "" && m.Name != "" {
			existing.Name = m.Name
		}
		if m.Username != "" {
			existing.Username = m.Username
		}
		if existing.AvatarURL == "" && m.AvatarURL != "" {
			existing.AvatarURL = m.AvatarURL
		}
		byUser[m.ID] = existing
	} else {
		byUser[m.ID] = m
	}
	s.touchMeta()
}

// RemoveMember deletes a guild member.
func (s *Store) RemoveMember(guild GuildID, user UserID) {
	delete(s.members[guild], user)
	s.touchMeta()
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

// ChannelRecipient returns a user from a direct or group DM's participant
// list. Guild channels do not carry recipients and return ok=false.
func (s *Store) ChannelRecipient(channel ChannelID, user UserID) (Member, bool) {
	c, ok := s.channels[channel]
	if !ok || c.Kind != ChannelDM {
		return Member{}, false
	}
	for _, recipient := range c.Recipients {
		if recipient.ID == user {
			return recipient, true
		}
	}
	return Member{}, false
}

// UpsertRole inserts or updates a guild role.
func (s *Store) UpsertRole(guild GuildID, r Role) {
	byRole := s.roles[guild]
	if byRole == nil {
		byRole = map[RoleID]Role{}
		s.roles[guild] = byRole
	}
	byRole[r.ID] = r
	s.touchMeta()
}

// RemoveRole deletes a guild role.
func (s *Store) RemoveRole(guild GuildID, role RoleID) {
	delete(s.roles[guild], role)
	s.touchMeta()
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
