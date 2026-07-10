package store

// Permission is a Discord permission bit set. Each guild role carries one (see
// [Role.Permissions]); a member's effective permissions are the bitwise-OR of
// the guild's @everyone role and every role the member holds, computed by
// [Store.MemberPermissions].
//
// The values mirror Discord's documented permission flags. Only the subset the
// client actually gates on is named; unknown bits are preserved through the OR
// so nothing is silently dropped.
type Permission uint64

// Discord permission bits. See
// https://discord.com/developers/docs/topics/permissions.
const (
	PermCreateInstantInvite Permission = 1 << 0
	PermKickMembers         Permission = 1 << 1
	PermBanMembers          Permission = 1 << 2
	// PermAdministrator grants every permission and bypasses channel overwrites.
	PermAdministrator   Permission = 1 << 3
	PermManageChannels  Permission = 1 << 4
	PermManageGuild     Permission = 1 << 5
	PermAddReactions    Permission = 1 << 6
	PermViewChannel     Permission = 1 << 10
	PermSendMessages    Permission = 1 << 11
	PermManageMessages  Permission = 1 << 13
	PermMentionEveryone Permission = 1 << 17
	PermManageNicknames Permission = 1 << 27
	PermManageRoles     Permission = 1 << 28
	// PermManageExpressions covers guild emojis and stickers.
	PermManageExpressions Permission = 1 << 30
	// PermManageThreads permits archiving, unarchiving, and locking threads a
	// member does not own.
	PermManageThreads Permission = 1 << 34
	// PermCreatePublicThreads permits creating threads on a channel.
	PermCreatePublicThreads Permission = 1 << 35
	// PermSendMessagesInThreads permits posting inside threads and forum posts.
	PermSendMessagesInThreads Permission = 1 << 38
)

// Has reports whether p grants perm. A set [PermAdministrator] bit grants every
// permission, so Has returns true for any perm when p is an administrator.
//
// Passing multiple bits requires all of them to be present (except under
// administrator, which always satisfies the check).
func (p Permission) Has(perm Permission) bool {
	if p&PermAdministrator != 0 {
		return true
	}
	return p&perm == perm
}

// CombinePermissions folds the @everyone base permissions and a member's role
// permissions into a single effective permission set. It is a pure helper used
// by [Store.MemberPermissions] and is exported for direct unit testing and reuse.
//
// The administrator bit is not expanded here; use [Permission.Has] to evaluate
// a specific permission, which honors administrator.
func CombinePermissions(base Permission, roles ...Permission) Permission {
	perms := base
	for _, r := range roles {
		perms |= r
	}
	return perms
}

// MemberPermissions returns a member's effective guild-level permissions: the
// @everyone role's permissions OR'd with each role the member holds. The guild
// owner receives all permissions.
//
// Channel-level overwrites are not applied; this is the guild baseline that the
// client uses to gate role-based actions (delete others' messages, manage
// roles/channels). Unknown guilds, members, or roles contribute nothing.
func (s *Store) MemberPermissions(guild GuildID, user UserID) Permission {
	if g, ok := s.guilds[guild]; ok && g.OwnerID != 0 && g.OwnerID == user {
		return Permission(^uint64(0))
	}
	var perms Permission
	// The @everyone role's ID equals the guild ID in Discord.
	if base, ok := s.roles[guild][RoleID(guild)]; ok {
		perms = base.Permissions
	}
	if m, ok := s.members[guild][user]; ok {
		for _, rid := range m.RoleIDs {
			if r, ok := s.roles[guild][rid]; ok {
				perms |= r.Permissions
			}
		}
	}
	return perms
}

// MemberCan reports whether a member has perm in a guild, honoring the
// administrator bit and guild ownership. It is the convenience wrapper the UI
// uses to enable or disable menu entries.
func (s *Store) MemberCan(guild GuildID, user UserID, perm Permission) bool {
	return s.MemberPermissions(guild, user).Has(perm)
}

// ApplyOverwrites folds a channel's permission overwrites over a base
// permission set following Discord's precedence: the @everyone (guild-ID) role
// overwrite first, then the OR of all matching role overwrites, then the
// member-specific overwrite last. Within each step deny bits clear before allow
// bits set. The administrator bit short-circuits everything (admins ignore
// overwrites), matching [Permission.Has].
//
// It is a pure helper — the guild ID doubles as the @everyone role ID, and the
// member's roles are passed explicitly — so the overwrite precedence is
// table-testable without a populated store.
func ApplyOverwrites(base Permission, everyoneRole uint64, memberRoles []uint64, member uint64, overwrites []PermissionOverwrite) Permission {
	if base&PermAdministrator != 0 {
		return base
	}
	perms := base

	// @everyone role overwrite.
	for _, o := range overwrites {
		if o.Role && o.ID == everyoneRole {
			perms &^= o.Deny
			perms |= o.Allow
		}
	}

	// Accumulate the member's role overwrites: all denies, then all allows.
	roleSet := make(map[uint64]bool, len(memberRoles))
	for _, r := range memberRoles {
		roleSet[r] = true
	}
	var allow, deny Permission
	for _, o := range overwrites {
		if o.Role && o.ID != everyoneRole && roleSet[o.ID] {
			allow |= o.Allow
			deny |= o.Deny
		}
	}
	perms &^= deny
	perms |= allow

	// Member-specific overwrite wins last.
	for _, o := range overwrites {
		if !o.Role && o.ID == member {
			perms &^= o.Deny
			perms |= o.Allow
		}
	}
	return perms
}

// ChannelPermissions returns a member's effective permissions in a specific
// channel: the guild-level baseline from [Store.MemberPermissions] with the
// channel's overwrites applied by [ApplyOverwrites]. Guild owners and
// administrators are unaffected by overwrites.
//
// For threads, overwrites are inherited from the parent channel (Discord does
// not store per-thread overwrites), so the parent's overwrites are used when
// the thread carries none of its own.
func (s *Store) ChannelPermissions(guild GuildID, user UserID, channel ChannelID) Permission {
	base := s.MemberPermissions(guild, user)
	if base&PermAdministrator != 0 {
		return base
	}
	c, ok := s.channels[channel]
	if !ok {
		return base
	}
	overwrites := c.Overwrites
	if len(overwrites) == 0 && c.Kind == ChannelThread && c.ParentID != 0 {
		if parent, ok := s.channels[c.ParentID]; ok {
			overwrites = parent.Overwrites
		}
	}
	var memberRoles []uint64
	if m, ok := s.members[guild][user]; ok {
		memberRoles = make([]uint64, len(m.RoleIDs))
		for i, r := range m.RoleIDs {
			memberRoles[i] = uint64(r)
		}
	}
	return ApplyOverwrites(base, uint64(guild), memberRoles, uint64(user), overwrites)
}

// ChannelCan reports whether a member holds perm in a specific channel, taking
// overwrites into account. It is the convenience wrapper the UI uses to gate the
// composer (SEND_MESSAGES) and channel-scoped actions.
func (s *Store) ChannelCan(guild GuildID, user UserID, channel ChannelID, perm Permission) bool {
	return s.ChannelPermissions(guild, user, channel).Has(perm)
}
