package store

import "testing"

// ── Permission.Has (pure) ────────────────────────────────────────────────────

func TestPermissionHas(t *testing.T) {
	tests := []struct {
		name string
		have Permission
		want Permission
		ok   bool
	}{
		{"exact single bit", PermManageMessages, PermManageMessages, true},
		{"missing bit", PermSendMessages, PermManageMessages, false},
		{"superset grants", PermManageMessages | PermManageRoles, PermManageRoles, true},
		{"admin grants anything", PermAdministrator, PermManageRoles, true},
		{"admin grants unnamed bit", PermAdministrator, Permission(1 << 40), true},
		{"empty grants nothing", 0, PermViewChannel, false},
		{"all bits required", PermManageMessages, PermManageMessages | PermManageRoles, false},
		{"all bits present", PermManageMessages | PermManageRoles, PermManageMessages | PermManageRoles, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.have.Has(tt.want); got != tt.ok {
				t.Errorf("Has(%b) on %b = %v, want %v", tt.want, tt.have, got, tt.ok)
			}
		})
	}
}

// ── CombinePermissions (pure) ────────────────────────────────────────────────

func TestCombinePermissions(t *testing.T) {
	tests := []struct {
		name  string
		base  Permission
		roles []Permission
		want  Permission
	}{
		{"base only", PermViewChannel, nil, PermViewChannel},
		{"no base no roles", 0, nil, 0},
		{
			name:  "roles OR into base",
			base:  PermViewChannel | PermSendMessages,
			roles: []Permission{PermManageMessages, PermManageRoles},
			want:  PermViewChannel | PermSendMessages | PermManageMessages | PermManageRoles,
		},
		{
			name:  "duplicate bits idempotent",
			base:  PermManageMessages,
			roles: []Permission{PermManageMessages, PermManageMessages},
			want:  PermManageMessages,
		},
		{
			name:  "admin bit preserved not expanded",
			base:  0,
			roles: []Permission{PermAdministrator},
			want:  PermAdministrator,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CombinePermissions(tt.base, tt.roles...); got != tt.want {
				t.Errorf("CombinePermissions = %b, want %b", got, tt.want)
			}
		})
	}
}

// ── Store.MemberPermissions / MemberCan ──────────────────────────────────────

// newPermStore builds a guild (id 1) with an @everyone role and two extra roles.
func newPermStore() *Store {
	s := New(0)
	s.UpsertGuild(Guild{ID: 1, Name: "g", OwnerID: 100})
	// @everyone role: ID == guild ID.
	s.UpsertRole(1, Role{ID: 1, Name: "@everyone", Permissions: PermViewChannel | PermSendMessages})
	s.UpsertRole(1, Role{ID: 20, Name: "mod", Position: 2, Permissions: PermManageMessages})
	s.UpsertRole(1, Role{ID: 30, Name: "admin", Position: 3, Permissions: PermAdministrator})
	return s
}

func TestMemberPermissions_EveryoneBaseline(t *testing.T) {
	s := newPermStore()
	s.UpsertMember(1, Member{ID: 200, Name: "plain"})

	got := s.MemberPermissions(1, 200)
	want := PermViewChannel | PermSendMessages
	if got != want {
		t.Errorf("baseline perms = %b, want %b", got, want)
	}
	if s.MemberCan(1, 200, PermManageMessages) {
		t.Error("plain member should not manage messages")
	}
}

func TestMemberPermissions_UnionsRoles(t *testing.T) {
	s := newPermStore()
	s.UpsertMember(1, Member{ID: 201, Name: "mod", RoleIDs: []RoleID{20}})

	if !s.MemberCan(1, 201, PermManageMessages) {
		t.Error("mod should manage messages via role 20")
	}
	if s.MemberCan(1, 201, PermManageRoles) {
		t.Error("mod should not manage roles")
	}
}

func TestMemberPermissions_AdministratorGrantsAll(t *testing.T) {
	s := newPermStore()
	s.UpsertMember(1, Member{ID: 202, Name: "admin", RoleIDs: []RoleID{30}})

	if !s.MemberCan(1, 202, PermManageRoles) {
		t.Error("admin should manage roles")
	}
	if !s.MemberCan(1, 202, PermManageExpressions) {
		t.Error("admin should have any permission")
	}
}

func TestMemberPermissions_OwnerHasEverything(t *testing.T) {
	s := newPermStore()
	// Owner (100) has no roles at all beyond @everyone.
	s.UpsertMember(1, Member{ID: 100, Name: "owner"})

	if !s.MemberCan(1, 100, PermManageRoles) {
		t.Error("owner should manage roles")
	}
	if !s.MemberCan(1, 100, PermBanMembers) {
		t.Error("owner should have every permission")
	}
}

func TestMemberPermissions_UnknownGuildOrMember(t *testing.T) {
	s := newPermStore()

	if got := s.MemberPermissions(999, 200); got != 0 {
		t.Errorf("unknown guild perms = %b, want 0", got)
	}
	if got := s.MemberPermissions(1, 999); got != PermViewChannel|PermSendMessages {
		t.Errorf("unknown member should still get @everyone, got %b", got)
	}
}

func TestMemberPermissions_ZeroOwnerIDDoesNotMatch(t *testing.T) {
	s := New(0)
	// Guild with unknown owner (OwnerID 0) must not grant everything to user 0.
	s.UpsertGuild(Guild{ID: 1, Name: "g"})
	s.UpsertRole(1, Role{ID: 1, Permissions: PermViewChannel})
	s.UpsertMember(1, Member{ID: 0, Name: "nobody"})

	if s.MemberCan(1, 0, PermManageRoles) {
		t.Error("user 0 must not be treated as owner when OwnerID is unset")
	}
}
