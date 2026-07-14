package app

import (
	"sync"
	"testing"

	"awesomeProject/internal/store"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
)

type fakeRoleManager struct {
	mu                     sync.Mutex
	create, modify, delete int
	done                   chan struct{}
}

func (f *fakeRoleManager) CreateRole(discord.GuildID, api.CreateRoleData) (*discord.Role, error) {
	f.mu.Lock()
	f.create++
	f.mu.Unlock()
	f.done <- struct{}{}
	return &discord.Role{}, nil
}
func (f *fakeRoleManager) ModifyRole(discord.GuildID, discord.RoleID, api.ModifyRoleData) (*discord.Role, error) {
	f.mu.Lock()
	f.modify++
	f.mu.Unlock()
	f.done <- struct{}{}
	return &discord.Role{}, nil
}
func (f *fakeRoleManager) DeleteRole(discord.GuildID, discord.RoleID, api.AuditLogReason) error {
	f.mu.Lock()
	f.delete++
	f.mu.Unlock()
	f.done <- struct{}{}
	return nil
}
func (f *fakeRoleManager) MoveRoles(discord.GuildID, api.MoveRolesData) ([]discord.Role, error) {
	return nil, nil
}

func TestRoleMutationsUseManagerOffThread(t *testing.T) {
	f := &fakeRoleManager{done: make(chan struct{}, 8)}
	a := &App{store: store.New(0), ui: syncPoster{}, roleManage: f}
	a.CreateRole(1, "new")
	a.RenameRole(1, 2, "renamed")
	a.SetRoleColor(1, 2, 0x112233)
	a.SetRoleHoist(1, 2, true)
	a.SetRoleMentionable(1, 2, true)
	a.DeleteRole(1, 2)
	for range 6 {
		waitSig(t, f.done)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.create != 1 || f.modify != 4 || f.delete != 1 {
		t.Fatalf("calls create=%d modify=%d delete=%d", f.create, f.modify, f.delete)
	}
}
