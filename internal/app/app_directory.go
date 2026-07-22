// Package app orchestrates the Discord session, the normalized store, and the TUI runtime.
package app

import (
	"awesomeProject/internal/store"
	"github.com/diamondburned/arikawa/v3/discord"
	"sync"
)

// LoadRoles fetches role definitions for a guild once per session. READY and
// GUILD_CREATE usually include roles, so this is primarily a guarded fallback.
func (a *App) LoadRoles(guild store.GuildID) {
	if a == nil || a.roles == nil || guild == 0 || guild == DirectMessagesGuildID {
		return
	}
	version, ok := a.beginRoleLoad(guild)
	if !ok {
		return
	}
	generation := a.store.GuildGeneration(guild)
	go a.loadRolesFrom(guild, generation, version)
}

// EnsureMemberDetail fetches a guild member's full record (including role IDs)
// when the store lacks it, then runs done on the UI goroutine so an open
// profile card can refresh. Message and REST history payloads carry only the
// author's global identity, so a profile opened from them would otherwise show
// an empty roles section until the user happens to post a live message.
func (a *App) EnsureMemberDetail(guild store.GuildID, user store.UserID, done func()) {
	if a == nil || a.memberDetail == nil || guild == 0 || guild == DirectMessagesGuildID || user == 0 {
		return
	}
	if m, ok := a.store.Member(guild, user); ok && len(m.RoleIDs) > 0 {
		return
	}
	go func() {
		member, err := a.memberDetail.Member(discord.GuildID(guild), discord.UserID(user))
		if err != nil || member == nil {
			return
		}
		a.ui.Post(func() {
			a.store.UpsertMember(guild, convertMember(*member, discord.GuildID(guild)))
			if done != nil {
				done()
			}
		})
	}()
}

// LoadGuilds fetches the cheap directory data needed to render the server and
// DM lists. It is guarded so startup or reconnect paths do not hammer REST.
func (a *App) LoadGuilds(limit uint) {
	if a == nil || a.dirs == nil {
		return
	}
	version, ok := a.beginGuildLoad()
	if !ok {
		return
	}
	snapshot := a.directoryRequestSnapshot()
	snapshot.gateVersion = version
	go a.loadGuildsFrom(limit, snapshot)
}

func (a *App) loadGuilds(limit uint) {
	a.loadGuildsFrom(limit, a.directoryRequestSnapshot())
}

func (a *App) loadGuildsFrom(limit uint, snapshot directoryRequestSnapshot) {
	guilds, guildErr := a.dirs.Guilds(limit)
	privateChannels, dmErr := a.dirs.PrivateChannels()
	if guildErr != nil && dmErr != nil {
		a.ui.Post(func() {
			// Any deletion invalidates the directory gate. Do not let this old
			// failure clear a newer request's pending state.
			if !a.directorySnapshotCurrent(snapshot, nil, nil) {
				a.finishGuildLoadVersion(snapshot.gateVersion, false)
				return
			}
			a.finishGuildLoad(false)
			if a.onError != nil {
				a.onError(guildErr)
			}
		})
		return
	}
	privateChannels = a.hydratePrivateChannels(privateChannels)
	a.ui.Post(func() {
		if !a.directorySnapshotCurrent(snapshot, guilds, privateChannels) {
			// A returned guild/channel was deleted or replaced while this directory
			// request (including DM detail hydration) was in flight. Finish only this
			// gate version so a generation-only rejection remains retryable without
			// disturbing a newer request after explicit invalidation.
			a.finishGuildLoadVersion(snapshot.gateVersion, false)
			return
		}
		for _, guild := range guilds {
			a.store.UpsertGuild(store.Guild{ID: store.GuildID(guild.ID), Name: guild.Name})
		}
		if len(privateChannels) > 0 {
			a.store.UpsertGuild(store.Guild{ID: DirectMessagesGuildID, Name: "Direct Messages"})
			for _, channel := range privateChannels {
				channel.GuildID = discord.GuildID(DirectMessagesGuildID)
				ingestPrivateChannel(a.store, channel)
			}
			a.markChannelsLoaded(DirectMessagesGuildID)
		}
		a.finishGuildLoad(true)
		if a.onReady != nil {
			a.onReady()
		}
	})
}

func (a *App) directorySnapshotCurrent(snapshot directoryRequestSnapshot, _ []discord.Guild, _ []discord.Channel) bool {
	a.resourceMu.Lock()
	currentVersion := a.guildsGate.version
	a.resourceMu.Unlock()
	if currentVersion != snapshot.gateVersion {
		return false
	}
	guilds := a.store.GuildGenerations()
	if len(guilds) != len(snapshot.guilds) {
		return false
	}
	for id, generation := range guilds {
		if snapshot.guilds[id] != generation {
			return false
		}
	}
	channels := a.store.ChannelGenerations()
	if len(channels) != len(snapshot.channels) {
		return false
	}
	for id, generation := range channels {
		if snapshot.channels[id] != generation {
			return false
		}
	}
	return true
}

// hydratePrivateChannels fills recipient data omitted by some user-session
// startup responses. Missing DMs are fetched with bounded concurrency and the
// enriched result is cached in the store with the rest of the directory.
func (a *App) hydratePrivateChannels(channels []discord.Channel) []discord.Channel {
	if a == nil || a.channelDetail == nil {
		return channels
	}
	sem := make(chan struct{}, dmHydrationConcurrency)
	var wg sync.WaitGroup
	for i := range channels {
		// Fetch full detail for any DM or group DM whose recipients were omitted.
		// Keying off the name here would skip named group DMs, which commonly
		// carry a custom name but still arrive without recipients, leaving their
		// @ mention menu empty.
		if len(channels[i].DMRecipients) > 0 {
			continue
		}
		if channels[i].Type != discord.DirectMessage && channels[i].Type != discord.GroupDM {
			continue
		}
		id := channels[i].ID
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			full, err := a.channelDetail.Channel(id)
			if err == nil && full != nil {
				channels[i] = *full
			}
		}(i)
	}
	wg.Wait()
	return channels
}

// LoadChannels fetches a guild's channel list once unless gateway already
// supplied it.
func (a *App) LoadChannels(guild store.GuildID) {
	if a == nil || a.chans == nil || guild == 0 || guild == DirectMessagesGuildID {
		return
	}
	version, ok := a.beginChannelLoad(guild)
	if !ok {
		return
	}
	generation := a.store.GuildGeneration(guild)
	go a.loadChannelsFrom(guild, generation, version)
}

// LoadForumMetadata refreshes a forum channel from Discord's channel endpoint.
// Gateway and guild-directory channel objects are not guaranteed to include
// available_tags, while the channel endpoint does.
func (a *App) LoadForumMetadata(channel store.ChannelID) {
	if a == nil || a.channelDetail == nil || channel == 0 {
		return
	}
	generation := a.store.ChannelGeneration(channel)
	a.resourceMu.Lock()
	a.ensureResourceMaps()
	if _, ok := a.forumMetaPending[channel]; ok {
		a.resourceMu.Unlock()
		return
	}
	a.forumMetaPending[channel] = generation
	a.resourceMu.Unlock()
	go func() {
		c, err := a.channelDetail.Channel(discord.ChannelID(channel))
		a.ui.Post(func() {
			if a.store.ChannelGeneration(channel) != generation {
				return
			}
			a.resourceMu.Lock()
			pendingGeneration, pending := a.forumMetaPending[channel]
			if !pending || pendingGeneration != generation {
				a.resourceMu.Unlock()
				return
			}
			delete(a.forumMetaPending, channel)
			a.resourceMu.Unlock()
			if err != nil || c == nil {
				if err != nil {
					a.reportError(err)
				}
				return
			}
			if existing, ok := a.store.Channel(channel); ok && c.GuildID == 0 {
				c.GuildID = discord.GuildID(existing.GuildID)
			}
			a.store.UpsertChannel(convertChannel(*c))
			if a.onChange != nil {
				a.onChange()
			}
		})
	}()
}

func (a *App) loadChannels(guild store.GuildID) {
	a.loadChannelsFrom(guild, a.store.GuildGeneration(guild), a.channelLoadVersion(guild))
}

func (a *App) loadChannelsFrom(guild store.GuildID, generation, version uint64) {
	channels, err := a.chans.Channels(discord.GuildID(guild))
	if err != nil {
		a.postIfCurrent(func() bool {
			return a.store.GuildGeneration(guild) == generation && a.channelLoadCurrent(guild, version)
		}, func() {
			a.finishChannelLoad(guild, false)
			a.reportErrorOnUI(err)
		})
		return
	}
	a.postIfCurrent(func() bool {
		return a.store.GuildGeneration(guild) == generation && a.channelLoadCurrent(guild, version)
	}, func() {
		for _, channel := range channels {
			channel.GuildID = discord.GuildID(guild)
			a.store.UpsertChannel(convertChannel(channel))
		}
		a.finishChannelLoad(guild, true)
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) loadRoles(guild store.GuildID) {
	a.loadRolesFrom(guild, a.store.GuildGeneration(guild), a.roleLoadVersion(guild))
}

func (a *App) loadRolesFrom(guild store.GuildID, generation, version uint64) {
	roles, err := a.roles.Roles(discord.GuildID(guild))
	if err != nil {
		a.postIfCurrent(func() bool {
			return a.store.GuildGeneration(guild) == generation && a.roleLoadCurrent(guild, version)
		}, func() {
			a.finishRoleLoad(guild, false)
			a.reportErrorOnUI(err)
		})
		return
	}
	a.postIfCurrent(func() bool {
		return a.store.GuildGeneration(guild) == generation && a.roleLoadCurrent(guild, version)
	}, func() {
		for _, role := range roles {
			a.store.UpsertRole(guild, convertRole(role))
		}
		a.finishRoleLoad(guild, true)
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) markRolesLoaded(guild store.GuildID) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.rolesGate.markLoaded(guild)
}

func (a *App) markChannelsLoaded(guild store.GuildID) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.channelsGate.markLoaded(guild)
}

func (a *App) beginGuildLoad() (uint64, bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	return a.guildsGate.beginVersion()
}

func (a *App) finishGuildLoad(ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.guildsGate.finish(ok)
}

func (a *App) finishGuildLoadVersion(version uint64, ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.guildsGate.finishVersion(version, ok)
}

func (a *App) beginHistoryLoad(channel store.ChannelID) (uint64, bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.historyGate.beginVersion(channel)
}

func (a *App) historyLoadCurrent(channel store.ChannelID, version uint64) bool {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.historyGate.version[channel] == version
}

func (a *App) finishHistoryLoad(channel store.ChannelID, ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.historyGate.finish(channel, ok)
}

func (a *App) beginOlderHistoryLoad(channel store.ChannelID) (uint64, bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.historyGate.beginOlderVersion(channel)
}

func (a *App) finishOlderHistory(channel store.ChannelID, exhausted bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.historyGate.finishOlder(channel, exhausted)
}

func (a *App) markHistoryExhausted(channel store.ChannelID) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.historyGate.markExhausted(channel)
}

func (a *App) beginRoleLoad(guild store.GuildID) (uint64, bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.rolesGate.beginVersion(guild)
}

func (a *App) roleLoadVersion(guild store.GuildID) uint64 {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.rolesGate.version[guild]
}

func (a *App) roleLoadCurrent(guild store.GuildID, version uint64) bool {
	return a.roleLoadVersion(guild) == version
}

func (a *App) finishRoleLoad(guild store.GuildID, ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.rolesGate.finish(guild, ok)
}

func (a *App) beginChannelLoad(guild store.GuildID) (uint64, bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.channelsGate.beginVersion(guild)
}

func (a *App) channelLoadVersion(guild store.GuildID) uint64 {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.channelsGate.version[guild]
}

func (a *App) channelLoadCurrent(guild store.GuildID, version uint64) bool {
	return a.channelLoadVersion(guild) == version
}

func (a *App) finishChannelLoad(guild store.GuildID, ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.channelsGate.finish(guild, ok)
}
