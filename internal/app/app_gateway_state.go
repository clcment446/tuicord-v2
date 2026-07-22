// Package app orchestrates the Discord session, the normalized store, and the TUI runtime.
package app

import (
	"awesomeProject/internal/store"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

// handleThreadUpsert writes a thread channel from a THREAD_CREATE/UPDATE event
// into the store, preserving the client's own membership when the event omits
// a ThreadMember (updates rarely echo it).
func (a *App) handleThreadUpsert(ch discord.Channel) {
	converted := convertChannel(ch)
	hadMember := ch.ThreadMember != nil
	a.ui.Post(func() {
		if !hadMember && converted.Thread != nil {
			if existing, ok := a.store.Channel(converted.ID); ok && existing.Thread != nil {
				converted.Thread.Joined = existing.Thread.Joined
			}
		}
		a.store.UpsertChannel(converted)
		if a.onChange != nil {
			a.onChange()
		}
	})
}

// handleThreadListSync ingests the bulk active-thread list Discord sends when
// the client gains access to a guild's channels.
func (a *App) handleThreadListSync(e *gateway.ThreadListSyncEvent) {
	guildID := store.GuildID(e.GuildID)
	threads := make([]store.Channel, 0, len(e.Threads))
	for _, t := range e.Threads {
		t.GuildID = e.GuildID
		threads = append(threads, convertChannel(t))
	}
	var parents []store.ChannelID
	if e.ChannelIDs != nil {
		parents = make([]store.ChannelID, len(e.ChannelIDs))
		for i, id := range e.ChannelIDs {
			parents[i] = store.ChannelID(id)
		}
	}
	joined := make(map[store.ChannelID]bool, len(e.Members))
	for _, m := range e.Members {
		joined[store.ChannelID(m.ID)] = true
	}
	a.ui.Post(func() {
		for i := range threads {
			if threads[i].Thread != nil && joined[threads[i].ID] {
				threads[i].Thread.Joined = true
			}
		}
		removed := a.store.SyncActiveThreads(guildID, parents, threads)
		readStateChanged := false
		for _, id := range removed {
			a.invalidateChannelLoads(id)
			if a.removeCachedReadState(id, guildID) {
				readStateChanged = true
			}
		}
		a.repairActiveChannel()
		if readStateChanged && a.onReadStateChange != nil {
			a.onReadStateChange()
		}
		a.markThreadsLoaded(guildID)
		if a.onChange != nil {
			a.onChange()
		}
	})
}

// handleThreadMembersUpdate updates the client's own membership when it is added
// to or removed from a thread.
func (a *App) handleThreadMembersUpdate(e *gateway.ThreadMembersUpdateEvent) {
	id := store.ChannelID(e.ID)
	addedMembers := append([]discord.ThreadMember(nil), e.AddedMembers...)
	removedMemberIDs := append([]discord.UserID(nil), e.RemovedMemberIDs...)
	a.ui.Post(func() {
		if a.selfID == 0 {
			return
		}
		added := false
		for _, member := range addedMembers {
			if store.UserID(member.UserID) == a.selfID {
				added = true
				break
			}
		}
		removed := false
		for _, userID := range removedMemberIDs {
			if store.UserID(userID) == a.selfID {
				removed = true
				break
			}
		}
		if !added && !removed {
			return
		}
		a.store.SetThreadJoined(id, added)
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) handleReady(e *gateway.ReadyEvent) {
	a.InvalidateCommandCache()
	guilds := e.Guilds
	privateChannels := e.PrivateChannels
	selfID := store.UserID(e.User.ID)
	sessionID := e.SessionID
	hasNitro := e.User.Nitro != discord.NoUserNitro
	var folders []store.GuildFolder
	if e.UserSettings != nil {
		folders = convertGuildFolders(e.UserSettings.GuildFolders)
	}
	// Snapshot READY's read markers synchronously (this handler runs on the
	// socket loop after ningen's own sync READY handler). Ningen retains pointers
	// into e.ReadStates and mutates them from later events, so we copy the values
	// out now rather than reading them back on the UI goroutine.
	readMarks := readMarksFromReady(e)
	a.ui.Post(func() {
		a.selfID = selfID
		a.publishStateSnapshot()
		a.sessionID = sessionID
		a.store.SetNitro(hasNitro)
		a.store.SetGuildFolders(folders)
		// READY guilds are not reconciled here: a user-session READY does not
		// reliably deliver the full guild/channel directory (hence the REST pull in
		// connect), so pruning against it would drop live entities. Per-guild
		// reconciliation happens on the authoritative GUILD_CREATE that follows.
		for i := range guilds {
			ingestGuild(a.store, &guilds[i])
			a.markRolesLoaded(store.GuildID(guilds[i].ID))
			if len(guilds[i].Channels) > 0 {
				a.markChannelsLoaded(store.GuildID(guilds[i].ID))
			}
		}
		if len(privateChannels) > 0 {
			a.store.UpsertGuild(store.Guild{ID: DirectMessagesGuildID, Name: "Direct Messages"})
		}
		for _, channel := range privateChannels {
			channel.GuildID = discord.GuildID(DirectMessagesGuildID)
			ingestPrivateChannel(a.store, channel)
		}
		// READY is authoritative for the connection's read-state generation.
		// Rebuild from the directory already in memory so startup is seeded and
		// a reconnect cannot retain entries from the previous session.
		a.replaceReadMarks(readMarks)
		a.resetReadStateCache()
		if a.onReady != nil {
			a.onReady()
		}
		a.emit("ready", nil)
	})
}

func (a *App) handleGuildCreate(e *gateway.GuildCreateEvent) {
	guild := *e
	a.ui.Post(func() {
		ingestGuild(a.store, &guild)
		if !guild.Unavailable {
			// A fresh GUILD_CREATE (notably on reconnect) is the authoritative
			// snapshot of this guild's channels and roles. Prune anything the store
			// still holds that the snapshot dropped while we were disconnected.
			a.reconcileGuildFromSnapshot(&guild)
			a.markRolesLoaded(store.GuildID(guild.ID))
			if len(guild.Channels) > 0 {
				a.markChannelsLoaded(store.GuildID(guild.ID))
			}
		}
		if a.onGuildChange != nil {
			// Guild refresh already rebuilds guild, channel, and member panels;
			// firing onChange too duplicated the expensive startup work.
			a.onGuildChange()
		}
	})
}

func (a *App) handleGuildUpdate(e *gateway.GuildUpdateEvent) {
	guild := store.Guild{
		ID:             store.GuildID(e.ID),
		Name:           e.Name,
		OwnerID:        store.UserID(e.OwnerID),
		RulesChannelID: store.ChannelID(e.RulesChannelID),
	}
	a.ui.Post(func() {
		a.store.UpsertGuild(guild)
		if a.onGuildChange != nil {
			a.onGuildChange()
		}
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) handleGuildDelete(e *gateway.GuildDeleteEvent) {
	id := store.GuildID(e.ID)
	unavailable := e.Unavailable
	a.ui.Post(func() {
		if unavailable {
			if !a.store.SetGuildUnavailable(id, true) {
				a.store.UpsertGuild(store.Guild{ID: id, Unavailable: true})
			}
		} else {
			a.invalidateGuildLoads(id)
			for _, channel := range a.store.Channels(id) {
				a.invalidateChannelLoads(channel.ID)
			}
			a.removeGuildReadState(id)
			a.store.RemoveGuild(id)
			if a.activeGuild == id {
				a.SetActive(0, 0)
			}
		}
		if a.onGuildChange != nil {
			a.onGuildChange()
		}
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) handleChannelUpsert(channel discord.Channel) {
	converted := convertChannel(channel)
	isDM := converted.Kind == store.ChannelDM
	if isDM {
		converted.GuildID = DirectMessagesGuildID
	}
	a.ui.Post(func() {
		if isDM {
			a.store.UpsertGuild(store.Guild{ID: DirectMessagesGuildID, Name: "Direct Messages"})
			wire := channel
			wire.GuildID = discord.GuildID(DirectMessagesGuildID)
			ingestPrivateChannel(a.store, wire)
		} else {
			a.store.UpsertChannel(converted)
		}
		if a.onGuildChange != nil && isDM {
			a.onGuildChange()
		}
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) handleChannelDelete(e *gateway.ChannelDeleteEvent) {
	id := store.ChannelID(e.ID)
	guildID := store.GuildID(e.GuildID)
	a.ui.Post(func() {
		if guildID == 0 {
			if channel, ok := a.store.Channel(id); ok {
				guildID = channel.GuildID
			}
		}
		a.invalidateGuildChannelLoads(guildID)
		readStateChanged := a.removeCachedReadState(id, guildID)
		for _, guild := range a.store.Guilds() {
			for _, channel := range a.store.Channels(guild.ID) {
				if channel.ID == id || channel.ParentID == id {
					a.invalidateChannelLoads(channel.ID)
					if channel.ParentID == id && a.removeCachedReadState(channel.ID, channel.GuildID) {
						readStateChanged = true
					}
				}
			}
		}
		a.invalidateChannelLoads(id)
		a.store.RemoveChannel(id)
		a.repairActiveChannel()
		if readStateChanged && a.onReadStateChange != nil {
			a.onReadStateChange()
		}
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) handleThreadDelete(e *gateway.ThreadDeleteEvent) {
	id := store.ChannelID(e.ID)
	guildID := store.GuildID(e.GuildID)
	parentID := store.ChannelID(e.ParentID)
	a.ui.Post(func() {
		if channel, ok := a.store.Channel(id); ok {
			if guildID == 0 {
				guildID = channel.GuildID
			}
			if parentID == 0 {
				parentID = channel.ParentID
			}
		}
		a.invalidateThreadLoad(guildID)
		a.invalidateArchivedLoad(parentID)
		a.invalidateChannelLoads(id)
		readStateChanged := a.removeCachedReadState(id, guildID)
		a.store.RemoveThread(id)
		a.repairActiveChannel()
		if readStateChanged && a.onReadStateChange != nil {
			a.onReadStateChange()
		}
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) invalidateGuildLoads(id store.GuildID) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.rolesGate.invalidate(id)
	a.channelsGate.invalidate(id)
	a.threadsGate.invalidate(id)
	a.guildsGate.invalidate()
}

func (a *App) invalidateGuildChannelLoads(id store.GuildID) {
	if id == 0 || id == DirectMessagesGuildID {
		return
	}
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.channelsGate.invalidate(id)
	a.threadsGate.invalidate(id)
}

func (a *App) invalidateThreadLoad(id store.GuildID) {
	if id == 0 || id == DirectMessagesGuildID {
		return
	}
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.threadsGate.invalidate(id)
}

func (a *App) invalidateRoleLoad(id store.GuildID) {
	if id == 0 || id == DirectMessagesGuildID {
		return
	}
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.rolesGate.invalidate(id)
}

func (a *App) invalidateArchivedLoad(id store.ChannelID) {
	if id == 0 {
		return
	}
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.archivedGate.invalidate(id)
	delete(a.archivedBefore, id)
}

func (a *App) invalidateChannelLoads(id store.ChannelID) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.historyGate.invalidate(id)
	a.archivedGate.invalidate(id)
	delete(a.archivedBefore, id)
	delete(a.forumMetaPending, id)
	// Guild/DM directory hydration can contain this channel. Let a fresh load
	// start, while its generation snapshot rejects the old completion.
	a.guildsGate.invalidate()
}

// reconcileGuildFromSnapshot prunes channels and roles the store still holds for
// a guild that a fresh GUILD_CREATE snapshot no longer lists — entities deleted
// while the client was disconnected, which never produce a delete event. Threads
// are intentionally left alone: the guild snapshot carries only active threads,
// so they are reconciled separately via THREAD_LIST_SYNC. A snapshot with no
// channels is treated as partial and never prunes channels.
func (a *App) reconcileGuildFromSnapshot(g *gateway.GuildCreateEvent) {
	if a == nil || a.store == nil || g == nil || g.Unavailable {
		return
	}
	guildID := store.GuildID(g.ID)
	readStateChanged := false
	if len(g.Channels) > 0 {
		channels := make(map[store.ChannelID]struct{}, len(g.Channels))
		for _, c := range g.Channels {
			channels[store.ChannelID(c.ID)] = struct{}{}
		}
		for _, c := range a.store.Channels(guildID) {
			if c.Thread != nil {
				continue
			}
			if _, ok := channels[c.ID]; ok {
				continue
			}
			a.invalidateChannelLoads(c.ID)
			if a.removeCachedReadState(c.ID, guildID) {
				readStateChanged = true
			}
			// RemoveChannel cascades this channel's child threads.
			a.store.RemoveChannel(c.ID)
		}
	}
	if len(g.Roles) > 0 {
		roles := make(map[store.RoleID]struct{}, len(g.Roles))
		for _, r := range g.Roles {
			roles[store.RoleID(r.ID)] = struct{}{}
		}
		rolesInvalidated := false
		for _, r := range a.store.Roles(guildID) {
			if _, ok := roles[r.ID]; ok {
				continue
			}
			if !rolesInvalidated {
				a.invalidateRoleLoad(guildID)
				rolesInvalidated = true
			}
			a.store.RemoveRole(guildID, r.ID)
		}
	}
	if readStateChanged && a.onReadStateChange != nil {
		a.onReadStateChange()
	}
	a.repairActiveChannel()
}

// repairActiveChannel clears a selection removed by channel or thread
// lifecycle events. It must run on the UI goroutine after the store mutation.
func (a *App) repairActiveChannel() {
	if a.activeChannel == 0 {
		return
	}
	if _, ok := a.store.Channel(a.activeChannel); !ok {
		a.SetActive(a.activeGuild, 0)
	}
}

func (a *App) handleMessageCreate(e *gateway.MessageCreateEvent) {
	msg := convertMessage(e.Message)
	member := (*discord.Member)(nil)
	if e.GuildID != 0 && e.Member != nil {
		copy := *e.Member
		copy.User = e.Author
		member = &copy
	} else if e.GuildID != 0 && e.Author.ID != 0 {
		member = &discord.Member{User: e.Author}
	}
	a.ui.Post(func() {
		if member != nil {
			converted := convertMember(*member, e.GuildID)
			if e.Member != nil {
				a.store.UpsertMember(store.GuildID(e.GuildID), converted)
			} else {
				a.store.RememberMemberIdentity(store.GuildID(e.GuildID), converted)
			}
		}
		// Reconcile an optimistic local echo when possible; otherwise append.
		appended := !a.store.ReplaceMessage(msg.Nonce, msg)
		pingsSelf := a.messagePingsSelf(e.Message)
		if appended && a.store.HasMessage(msg.ChannelID, msg.ID) {
			// The gateway can redeliver a MESSAGE_CREATE. It carries no nonce match,
			// so it looks like a fresh append; appending would duplicate the message
			// and double-count unread/pings. It is already stored — ignore it.
			appended = false
		} else if appended {
			a.store.AppendMessage(msg)
			if msg.ChannelID != a.activeChannel && msg.AuthorID != a.selfID {
				a.store.IncrementUnread(msg.ChannelID)
				if pingsSelf {
					a.store.IncrementPing(msg.ChannelID)
				}
			}
		}
		if appended && pingsSelf && msg.AuthorID != 0 && msg.AuthorID != a.selfID && a.onIncomingMessage != nil {
			a.onIncomingMessage(msg)
		}
		if a.onChange != nil {
			a.onChange()
		}
		a.emit("message.create", pluginMessagePayload(e.Message, uint64(e.GuildID), e.Author.Bot))
	})
}

// messagePingsSelf classifies a gateway message using Discord's structured
// mention fields. This deliberately runs before the optimistic-message swap is
// counted, so local echoes cannot produce false sidebar notifications.
func (a *App) messagePingsSelf(message discord.Message) bool {
	channel, knownChannel := a.store.Channel(store.ChannelID(message.ChannelID))
	if knownChannel && channel.Kind == store.ChannelDM {
		return true
	}
	if message.MentionEveryone {
		return true
	}
	for _, mentioned := range message.Mentions {
		if store.UserID(mentioned.ID) == a.selfID && a.selfID != 0 {
			return true
		}
	}
	if message.GuildID == 0 || a.selfID == 0 {
		return false
	}
	self, ok := a.store.Member(store.GuildID(message.GuildID), a.selfID)
	if !ok {
		return false
	}
	for _, mentionedRole := range message.MentionRoleIDs {
		for _, role := range self.RoleIDs {
			if store.RoleID(mentionedRole) == role {
				return true
			}
		}
	}
	return false
}

// handleMessageUpdate patches an existing message in place. Discord unfurls link
// embeds after the initial MESSAGE_CREATE and dispatches them as an update, so
// the rich content (embeds, attachments, components) must be merged by ID.
func (a *App) handleMessageUpdate(e *gateway.MessageUpdateEvent) {
	patch := convertMessage(e.Message)
	channel := patch.ChannelID
	id := patch.ID
	var fields *gateway.MessageUpdateFields
	if e.Fields != nil {
		copied := *e.Fields
		fields = &copied
	}
	componentTreeLegacy := convertComponentTree(e.Message.Components, false)
	componentTreeV2 := convertComponentTree(e.Message.Components, true)
	guildID := uint64(e.GuildID)
	a.ui.Post(func() {
		a.store.UpdateMessage(channel, id, func(m *store.Message) {
			allFields := fields == nil
			if allFields || fields.Content {
				m.Content = patch.Content
			}
			if allFields || fields.Flags {
				m.Flags = patch.Flags
			}
			if allFields || fields.Attachments {
				m.Attachments = patch.Attachments
			}
			if allFields || fields.Embeds {
				m.Embeds = patch.Embeds
			}
			if allFields || fields.Stickers {
				m.Stickers = patch.Stickers
			}
			if allFields || fields.Components {
				m.Components = patch.Components
				m.ComponentTree = componentTreeLegacy
				if m.Flags&uint64(discord.IsComponentsV2) != 0 {
					m.ComponentTree = componentTreeV2
				}
			}
			if allFields || fields.Pinned {
				m.Pinned = patch.Pinned
			}
			// Reference data is immutable, but full-message updates (embed
			// unfurls, edits) re-deliver it; keep whichever side has it so a
			// partial payload never wipes an existing reply or forward.
			if patch.Reply != nil {
				m.Reply = patch.Reply
			}
			if len(patch.Forwards) > 0 {
				m.Forwards = patch.Forwards
			}
		})
		if a.onChange != nil {
			a.onChange()
		}
		a.emit("message.update", pluginMessagePayload(e.Message, guildID, e.Author.Bot))
	})
}

func (a *App) handleMessageDelete(e *gateway.MessageDeleteEvent) {
	channel := store.ChannelID(e.ChannelID)
	id := store.MessageID(e.ID)
	a.ui.Post(func() {
		a.store.RemoveMessage(channel, id)
		if a.onChange != nil {
			a.onChange()
		}
		a.emit("message.delete", map[string]any{
			"id":         uint64(id),
			"channel_id": uint64(channel),
		})
	})
}

func (a *App) handleMessageDeleteBulk(e *gateway.MessageDeleteBulkEvent) {
	channel := store.ChannelID(e.ChannelID)
	ids := append([]discord.MessageID(nil), e.IDs...)
	a.ui.Post(func() {
		for _, id := range ids {
			a.store.RemoveMessage(channel, store.MessageID(id))
		}
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) handleReactionAdd(e *gateway.MessageReactionAddEvent) {
	channel := store.ChannelID(e.ChannelID)
	id := store.MessageID(e.MessageID)
	userID := uint64(e.UserID)
	emoji := e.Emoji.Name
	emojiID := uint64(e.Emoji.ID)
	animated := e.Emoji.Animated
	a.ui.Post(func() {
		a.store.AddReaction(channel, id, store.Reaction{
			EmojiName: emoji,
			EmojiID:   emojiID,
			Animated:  animated,
			Count:     1,
			Me:        store.UserID(userID) == a.selfID && a.selfID != 0,
		})
		if a.onChange != nil {
			a.onChange()
		}
		a.emit("reaction.add", map[string]any{
			"channel_id": uint64(channel),
			"message_id": uint64(id),
			"user_id":    userID,
			"emoji":      emoji,
		})
	})
}

func (a *App) handleReactionRemove(e *gateway.MessageReactionRemoveEvent) {
	channel := store.ChannelID(e.ChannelID)
	id := store.MessageID(e.MessageID)
	name := e.Emoji.Name
	emojiID := uint64(e.Emoji.ID)
	userID := uint64(e.UserID)
	a.ui.Post(func() {
		me := store.UserID(userID) == a.selfID && a.selfID != 0
		a.store.RemoveReaction(channel, id, name, emojiID, me)
		if a.onChange != nil {
			a.onChange()
		}
		a.emit("reaction.remove", map[string]any{
			"channel_id": uint64(channel),
			"message_id": uint64(id),
			"user_id":    userID,
			"emoji":      name,
		})
	})
}

func (a *App) handleReactionRemoveEmoji(e *gateway.MessageReactionRemoveEmojiEvent) {
	channel := store.ChannelID(e.ChannelID)
	id := store.MessageID(e.MessageID)
	name := e.Emoji.Name
	emojiID := uint64(e.Emoji.ID)
	a.ui.Post(func() {
		a.store.RemoveReactionEmoji(channel, id, name, emojiID)
		if a.onChange != nil {
			a.onChange()
		}
		a.emit("reaction.remove_emoji", map[string]any{
			"channel_id": uint64(channel),
			"message_id": uint64(id),
			"emoji":      name,
		})
	})
}

func (a *App) handleReactionRemoveAll(e *gateway.MessageReactionRemoveAllEvent) {
	channel := store.ChannelID(e.ChannelID)
	id := store.MessageID(e.MessageID)
	a.ui.Post(func() {
		a.store.UpdateMessage(channel, id, func(m *store.Message) {
			m.Reactions = nil
		})
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) handleMembersChunk(e *gateway.GuildMembersChunkEvent) {
	guildID := store.GuildID(e.GuildID)
	members := append([]discord.Member(nil), e.Members...)
	a.ui.Post(func() {
		for _, member := range members {
			a.store.UpsertMember(guildID, convertMember(member, e.GuildID))
		}
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) handleMemberUpsert(guild discord.GuildID, member discord.Member) {
	guildID := store.GuildID(guild)
	a.ui.Post(func() {
		a.store.UpsertMember(guildID, convertMember(member, guild))
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) handleRoleUpsert(guild discord.GuildID, role discord.Role) {
	guildID := store.GuildID(guild)
	a.ui.Post(func() {
		a.store.UpsertRole(guildID, convertRole(role))
		a.markRolesLoaded(guildID)
		if a.onChange != nil {
			a.onChange()
		}
	})
}
