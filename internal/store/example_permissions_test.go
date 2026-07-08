package store_test

import (
	"fmt"

	"awesomeProject/internal/store"
)

// A member's effective permissions are the @everyone role OR'd with each role
// they hold; the administrator bit and guild ownership grant everything.
func ExampleStore_MemberPermissions() {
	s := store.New(0)
	s.UpsertGuild(store.Guild{ID: 1, Name: "gophers", OwnerID: 100})

	// @everyone role shares the guild ID and sets the baseline.
	s.UpsertRole(1, store.Role{ID: 1, Name: "@everyone",
		Permissions: store.PermViewChannel | store.PermSendMessages})
	s.UpsertRole(1, store.Role{ID: 2, Name: "mod",
		Permissions: store.PermManageMessages})

	s.UpsertMember(1, store.Member{ID: 200, Name: "alice", RoleIDs: []store.RoleID{2}})

	fmt.Println("alice can delete messages:", s.MemberCan(1, 200, store.PermManageMessages))
	fmt.Println("alice can manage roles:", s.MemberCan(1, 200, store.PermManageRoles))
	fmt.Println("owner can manage roles:", s.MemberCan(1, 100, store.PermManageRoles))
	// Output:
	// alice can delete messages: true
	// alice can manage roles: false
	// owner can manage roles: true
}
