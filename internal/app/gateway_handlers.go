package app

import (
	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3/states/read"
)

func (a *App) registerGatewayLifecycleHandlers() {
	a.handle.AddHandler(func(e *gateway.ChannelUnreadUpdateEvent) {
		a.handleChannelUnreadUpdate(e)
	})
	a.handle.AddHandler(func(e *read.UpdateEvent) {
		a.handleReadStateUpdate(e)
	})
	a.handle.AddHandler(func(e *gateway.ReadyEvent) {
		a.handleReady(e)
	})

	a.handle.AddHandler(func(e *gateway.GuildCreateEvent) {
		a.handleGuildCreate(e)
	})

	a.handle.AddHandler(func(e *gateway.GuildUpdateEvent) {
		a.handleGuildUpdate(e)
	})

	a.handle.AddHandler(func(e *gateway.GuildDeleteEvent) {
		a.handleGuildDelete(e)
	})

	a.handle.AddHandler(func(e *gateway.ChannelCreateEvent) {
		a.handleChannelUpsert(e.Channel)
	})

	a.handle.AddHandler(func(e *gateway.ChannelUpdateEvent) {
		a.handleChannelUpsert(e.Channel)
	})

	a.handle.AddHandler(func(e *gateway.ChannelDeleteEvent) {
		a.handleChannelDelete(e)
	})

	a.handle.AddHandler(func(e *gateway.GuildEmojisUpdateEvent) {
		guildID := store.GuildID(e.GuildID)
		emojis := convertGuildEmojis(e.Emojis)
		a.ui.Post(func() {
			a.store.SetGuildEmojis(guildID, emojis)
			if a.onChange != nil {
				a.onChange()
			}
		})
	})

	a.handle.AddHandler(func(e *gateway.UserSettingsUpdateEvent) {
		folders := convertGuildFolders(e.GuildFolders)
		a.ui.Post(func() {
			a.store.SetGuildFolders(folders)
			if a.onChange != nil {
				a.onChange()
			}
		})
	})
}

func (a *App) handleChannelUnreadUpdate(e *gateway.ChannelUnreadUpdateEvent) {
	if a == nil || e == nil || len(e.ChannelUnreadUpdates) == 0 {
		return
	}
	guild := store.GuildID(e.GuildID)
	updates := make(map[store.ChannelID]UnreadStatus, len(e.ChannelUnreadUpdates))
	for _, update := range e.ChannelUnreadUpdates {
		channel := store.ChannelID(update.ID)
		status := Unread
		if a.channelMutedLocal(channel) {
			status = Read
		}
		updates[channel] = status
	}
	// Do not call ningen.MarkUnread here. It emits one asynchronous UpdateEvent
	// per entry, turning one bulk startup dispatch into N+1 UI refreshes.
	a.cacheReadStateBatch(guild, updates)
	a.ui.Post(func() {
		if a.onReadStateChange != nil {
			a.onReadStateChange()
		}
	})
}

func (a *App) handleReadStateUpdate(e *read.UpdateEvent) {
	if a == nil || e == nil {
		return
	}
	channel := store.ChannelID(e.ChannelID)
	guild := store.GuildID(e.GuildID)
	// UpdateEvent already contains ningen's scalar unread result. Applying mute
	// state locally preserves the useful semantics without its permission-aware
	// helper performing REST fallback on this callback goroutine.
	status := Read
	if e.ReadState.MentionCount > 0 {
		status = Mentioned
	} else if e.Unread && !a.channelMutedLocal(channel) {
		status = Unread
	}
	a.cacheReadState(channel, guild, status)
	a.ui.Post(func() {
		if status == Read && a.store != nil {
			a.store.ClearUnread(channel)
		}
		if a.onReadStateChange != nil {
			a.onReadStateChange()
		}
	})
}

func (a *App) registerGatewayMessageHandlers() {
	a.handle.AddHandler(func(e *gateway.MessageCreateEvent) {
		a.handleMessageCreate(e)
	})

	a.handle.AddHandler(func(e *gateway.MessageUpdateEvent) {
		a.handleMessageUpdate(e)
	})

	a.handle.AddHandler(func(e *gateway.MessageDeleteEvent) {
		a.handleMessageDelete(e)
	})

	a.handle.AddHandler(func(e *gateway.MessageDeleteBulkEvent) {
		a.handleMessageDeleteBulk(e)
	})

	a.handle.AddHandler(func(e *gateway.MessageReactionAddEvent) {
		a.handleReactionAdd(e)
	})

	a.handle.AddHandler(func(e *gateway.MessageReactionRemoveEvent) {
		a.handleReactionRemove(e)
	})

	a.handle.AddHandler(func(e *gateway.MessageReactionRemoveAllEvent) {
		a.handleReactionRemoveAll(e)
	})
}

func (a *App) registerGatewayMemberHandlers() {
	a.handle.AddHandler(func(e *gateway.GuildMembersChunkEvent) {
		a.handleMembersChunk(e)
	})

	a.handle.AddHandler(func(e *gateway.GuildMemberAddEvent) {
		a.handleMemberUpsert(e.GuildID, e.Member)
	})

	a.handle.AddHandler(func(e *gateway.GuildMemberUpdateEvent) {
		member := discord.Member{User: e.User}
		e.UpdateMember(&member)
		a.handleMemberUpsert(e.GuildID, member)
	})

	a.handle.AddHandler(func(e *gateway.GuildMemberRemoveEvent) {
		guildID := store.GuildID(e.GuildID)
		userID := store.UserID(e.User.ID)
		a.ui.Post(func() {
			a.store.RemoveMember(guildID, userID)
			if a.onChange != nil {
				a.onChange()
			}
		})
	})

	a.handle.AddHandler(func(e *gateway.GuildRoleCreateEvent) {
		a.handleRoleUpsert(e.GuildID, e.Role)
	})

	a.handle.AddHandler(func(e *gateway.GuildRoleUpdateEvent) {
		a.handleRoleUpsert(e.GuildID, e.Role)
	})

	a.handle.AddHandler(func(e *gateway.GuildRoleDeleteEvent) {
		guildID := store.GuildID(e.GuildID)
		roleID := store.RoleID(e.RoleID)
		a.ui.Post(func() {
			a.invalidateRoleLoad(guildID)
			a.store.RemoveRole(guildID, roleID)
			if a.onChange != nil {
				a.onChange()
			}
		})
	})
}

func (a *App) registerGatewayThreadHandlers() {
	a.handle.AddHandler(func(e *gateway.ThreadCreateEvent) {
		a.handleThreadUpsert(e.Channel)
	})

	a.handle.AddHandler(func(e *gateway.ThreadUpdateEvent) {
		a.handleThreadUpsert(e.Channel)
	})

	a.handle.AddHandler(func(e *gateway.ThreadDeleteEvent) {
		a.handleThreadDelete(e)
	})

	a.handle.AddHandler(func(e *gateway.ThreadListSyncEvent) {
		a.handleThreadListSync(e)
	})

	a.handle.AddHandler(func(e *gateway.ThreadMemberUpdateEvent) {
		id := store.ChannelID(e.ThreadMember.ID)
		a.ui.Post(func() {
			// A ThreadMemberUpdate for the current user means we joined; leaving
			// arrives via ThreadMembersUpdate/RemovedMemberIDs handled below.
			a.store.SetThreadJoined(id, true)
			if a.onChange != nil {
				a.onChange()
			}
		})
	})

	a.handle.AddHandler(func(e *gateway.ThreadMembersUpdateEvent) {
		a.handleThreadMembersUpdate(e)
	})
}
