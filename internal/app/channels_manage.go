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
	a.runInBackground(func() error {
		_, err := a.channelManage.CreateChannel(discord.GuildID(g), api.CreateChannelData{Name: strings.TrimSpace(name), Type: discord.GuildText})
		return err
	})
}
func (a *App) RenameChannel(id store.ChannelID, name string) {
	if a == nil || a.channelManage == nil || strings.TrimSpace(name) == "" {
		return
	}
	a.runInBackground(func() error {
		return a.channelManage.ModifyChannel(discord.ChannelID(id), api.ModifyChannelData{Name: strings.TrimSpace(name)})
	})
}
func (a *App) DeleteChannel(id store.ChannelID) {
	if a == nil || a.channelManage == nil || id == 0 {
		return
	}
	a.runInBackground(func() error {
		return a.channelManage.DeleteChannel(discord.ChannelID(id), api.AuditLogReason(""))
	})
}
func (a *App) MoveChannel(g store.GuildID, id store.ChannelID, position int) {
	if a == nil || a.channelManage == nil || id == 0 {
		return
	}
	a.runInBackground(func() error {
		data := api.MoveChannelsData{Channels: []api.MoveChannelData{{ID: discord.ChannelID(id), Position: option.NewInt(position)}}}
		return a.channelManage.MoveChannels(discord.GuildID(g), data)
	})
}
