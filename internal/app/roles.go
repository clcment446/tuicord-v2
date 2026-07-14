package app

import (
	"strings"

	"awesomeProject/internal/store"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
)

// CreateRole creates a guild role off the UI thread; gateway events reconcile it.
func (a *App) CreateRole(guild store.GuildID, name string) {
	if a == nil || a.roleManage == nil || guild == 0 || strings.TrimSpace(name) == "" {
		return
	}
	go func() {
		if _, err := a.roleManage.CreateRole(discord.GuildID(guild), api.CreateRoleData{Name: strings.TrimSpace(name)}); err != nil {
			a.reportError(err)
		}
	}()
}

// RenameRole changes a role name off-thread.
func (a *App) RenameRole(guild store.GuildID, role store.RoleID, name string) {
	if strings.TrimSpace(name) == "" {
		return
	}
	a.modifyRole(guild, role, api.ModifyRoleData{Name: option.NewNullableString(strings.TrimSpace(name))})
}

func (a *App) SetRoleColor(guild store.GuildID, role store.RoleID, color uint32) {
	a.modifyRole(guild, role, api.ModifyRoleData{Color: discord.Color(color)})
}
func (a *App) SetRoleHoist(guild store.GuildID, role store.RoleID, value bool) {
	v := option.NullableFalse
	if value {
		v = option.NullableTrue
	}
	a.modifyRole(guild, role, api.ModifyRoleData{Hoist: v})
}
func (a *App) SetRoleMentionable(guild store.GuildID, role store.RoleID, value bool) {
	v := option.NullableFalse
	if value {
		v = option.NullableTrue
	}
	a.modifyRole(guild, role, api.ModifyRoleData{Mentionable: v})
}
func (a *App) modifyRole(guild store.GuildID, role store.RoleID, data api.ModifyRoleData) {
	if a == nil || a.roleManage == nil || guild == 0 || role == 0 {
		return
	}
	go func() {
		if _, err := a.roleManage.ModifyRole(discord.GuildID(guild), discord.RoleID(role), data); err != nil {
			a.reportError(err)
		}
	}()
}
func (a *App) DeleteRole(guild store.GuildID, role store.RoleID) {
	if a == nil || a.roleManage == nil || guild == 0 || role == 0 {
		return
	}
	go func() {
		if err := a.roleManage.DeleteRole(discord.GuildID(guild), discord.RoleID(role), api.AuditLogReason("")); err != nil {
			a.reportError(err)
		}
	}()
}

func (a *App) MoveRole(guild store.GuildID, role store.RoleID, position int) {
	if a == nil || a.roleManage == nil || role == 0 {
		return
	}
	go func() {
		_, err := a.roleManage.MoveRoles(discord.GuildID(guild), api.MoveRolesData{Roles: []api.MoveRoleData{{ID: discord.RoleID(role), Position: option.NewNullableInt(position)}}})
		if err != nil {
			a.reportError(err)
		}
	}()
}
