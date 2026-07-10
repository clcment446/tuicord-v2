package store

import "testing"

func TestApplyOverwritesEveryoneDenySend(t *testing.T) {
	const everyone = 1
	base := PermViewChannel | PermSendMessages
	ows := []PermissionOverwrite{
		{ID: everyone, Role: true, Deny: PermSendMessages},
	}
	got := ApplyOverwrites(base, everyone, nil, 42, ows)
	if got.Has(PermSendMessages) {
		t.Error("@everyone deny should remove SEND_MESSAGES")
	}
	if !got.Has(PermViewChannel) {
		t.Error("VIEW_CHANNEL should survive")
	}
}

func TestApplyOverwritesRoleAllowBeatsEveryoneDeny(t *testing.T) {
	const everyone, modRole = 1, 5
	base := PermViewChannel
	ows := []PermissionOverwrite{
		{ID: everyone, Role: true, Deny: PermSendMessages},
		{ID: modRole, Role: true, Allow: PermSendMessages},
	}
	// A member holding modRole regains SEND despite the @everyone deny, because
	// role allows are applied after the @everyone overwrite.
	got := ApplyOverwrites(base, everyone, []uint64{modRole}, 42, ows)
	if !got.Has(PermSendMessages) {
		t.Error("role allow should override @everyone deny")
	}
}

func TestApplyOverwritesMemberDenyWinsLast(t *testing.T) {
	const everyone, modRole = 1, 5
	const member = 42
	base := PermViewChannel | PermSendMessages
	ows := []PermissionOverwrite{
		{ID: modRole, Role: true, Allow: PermSendMessages},
		{ID: member, Role: false, Deny: PermSendMessages},
	}
	got := ApplyOverwrites(base, everyone, []uint64{modRole}, member, ows)
	if got.Has(PermSendMessages) {
		t.Error("member-specific deny must win over role allow")
	}
}

func TestApplyOverwritesAdminIgnoresAll(t *testing.T) {
	const everyone = 1
	base := PermAdministrator
	ows := []PermissionOverwrite{{ID: everyone, Role: true, Deny: PermSendMessages}}
	got := ApplyOverwrites(base, everyone, nil, 42, ows)
	if !got.Has(PermSendMessages) {
		t.Error("administrator must bypass overwrites")
	}
}

func TestChannelPermissionsInheritsParentOverwrites(t *testing.T) {
	s := New(0)
	const guild GuildID = 1
	const everyone = RoleID(guild)
	s.UpsertGuild(Guild{ID: guild})
	s.UpsertRole(guild, Role{ID: everyone, Permissions: PermViewChannel | PermSendMessages})
	s.UpsertMember(guild, Member{ID: 42})
	s.UpsertChannel(Channel{
		ID: 100, GuildID: guild, Kind: ChannelText,
		Overwrites: []PermissionOverwrite{{ID: uint64(everyone), Role: true, Deny: PermSendMessages}},
	})
	s.UpsertThread(Channel{ID: 10, GuildID: guild, ParentID: 100, Thread: &ThreadMeta{}})

	if s.ChannelCan(guild, 42, 100, PermSendMessages) {
		t.Error("parent channel denies SEND to @everyone")
	}
	if s.ChannelCan(guild, 42, 10, PermSendMessages) {
		t.Error("thread should inherit parent's SEND deny")
	}
}

func TestChannelPermissionsOwnerAllowed(t *testing.T) {
	s := New(0)
	const guild GuildID = 1
	s.UpsertGuild(Guild{ID: guild, OwnerID: 42})
	s.UpsertChannel(Channel{
		ID: 100, GuildID: guild, Kind: ChannelText,
		Overwrites: []PermissionOverwrite{{ID: uint64(guild), Role: true, Deny: PermSendMessages}},
	})
	if !s.ChannelCan(guild, 42, 100, PermSendMessages) {
		t.Error("guild owner must bypass channel overwrites")
	}
}
