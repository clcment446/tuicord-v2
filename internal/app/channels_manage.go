package app

import (
	"awesomeProject/internal/store"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
	"strings"
)

func (a *App) CreateTextChannel(g store.GuildID, name string) {
	if a == nil || a.channelManage == nil || strings.TrimSpace(name) == "" {
		return
	}
	go func() {
		if _, err := a.channelManage.CreateChannel(discord.GuildID(g), api.CreateChannelData{Name: strings.TrimSpace(name), Type: discord.GuildText}); err != nil {
			a.reportError(err)
		}
	}()
}
func (a *App) RenameChannel(id store.ChannelID, name string) {
	if a == nil || a.channelManage == nil || strings.TrimSpace(name) == "" {
		return
	}
	go func() {
		if err := a.channelManage.ModifyChannel(discord.ChannelID(id), api.ModifyChannelData{Name: strings.TrimSpace(name)}); err != nil {
			a.reportError(err)
		}
	}()
}
func (a *App) DeleteChannel(id store.ChannelID) {
	if a == nil || a.channelManage == nil || id == 0 {
		return
	}
	go func() {
		if err := a.channelManage.DeleteChannel(discord.ChannelID(id), api.AuditLogReason("")); err != nil {
			a.reportError(err)
		}
	}()
}
func (a *App) MoveChannel(g store.GuildID, id store.ChannelID, position int) {
	if a == nil || a.channelManage == nil || id == 0 {
		return
	}
	go func() {
		data := api.MoveChannelsData{Channels: []api.MoveChannelData{{ID: discord.ChannelID(id), Position: option.NewInt(position)}}}
		if err := a.channelManage.MoveChannels(discord.GuildID(g), data); err != nil {
			a.reportError(err)
		}
	}()
}
