package matrixapp

import (
	"maunium.net/go/mautrix/id"

	"awesomeProject/internal/backend"
	"awesomeProject/internal/store"
)

// roomInfoLocked returns the accumulated info for a room, creating it (and its
// interned channel ID) on first use.
func (a *App) roomInfoLocked(roomID id.RoomID) *roomInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	if info, ok := a.rooms[roomID]; ok {
		return info
	}
	channelID := store.ChannelID(a.ids.intern(string(roomID)))
	info := &roomInfo{channelID: channelID}
	a.rooms[roomID] = info
	a.roomByChannel[channelID] = roomID
	return info
}

// channelFor returns the store channel ID for a room, interning if needed.
func (a *App) channelFor(roomID id.RoomID) store.ChannelID {
	return a.roomInfoLocked(roomID).channelID
}

// guildFor computes the sidebar guild a room belongs to.
func (a *App) guildFor(roomID id.RoomID) store.GuildID {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.guildForLocked(roomID)
}

func (a *App) guildForLocked(roomID id.RoomID) store.GuildID {
	if a.directRooms[roomID] {
		return backend.DirectMessagesGuildID
	}
	if space, ok := a.childToSpace[roomID]; ok {
		return store.GuildID(a.ids.intern(string(space)))
	}
	return UngroupedRoomsGuildID
}

// syncRoomEntry (re)creates the store guild + channel for a room from current
// knowledge. Spaces become guilds and are not themselves listed as channels.
// Called after any state change that could affect placement or naming.
func (a *App) syncRoomEntry(roomID id.RoomID) {
	a.mu.Lock()
	info := a.rooms[roomID]
	if info == nil {
		a.mu.Unlock()
		return
	}
	isSpace := info.isSpace
	name := info.name
	channelID := info.channelID
	isDM := a.directRooms[roomID]
	guild := a.guildForLocked(roomID)
	spaceGuild := store.GuildID(a.ids.intern(string(roomID)))
	a.mu.Unlock()

	a.ui.Post(func() {
		if isSpace {
			// A space is a guild, never a channel. Ensure the guild exists and
			// drop any channel row previously created for it.
			a.store.UpsertGuild(store.Guild{ID: spaceGuild, Name: displayOr(name, "Space")})
			a.store.RemoveChannel(channelID)
			a.fireGuildChange()
			return
		}
		a.ensureGuildRow(guild)
		kind := store.ChannelText
		chanName := name
		if isDM {
			kind = store.ChannelDM
			if chanName == "" {
				chanName = a.roomDisplayName(roomID)
			}
		}
		if chanName == "" {
			chanName = a.roomDisplayName(roomID)
		}
		a.store.UpsertChannel(store.Channel{
			ID:      channelID,
			GuildID: guild,
			Name:    chanName,
			Kind:    kind,
		})
		a.fireGuildChange()
		a.markReadyOnce()
	})
}

// ensureGuildRow upserts one of the synthetic container guilds (DMs, Rooms) or a
// space guild so it is visible in the sidebar. Must run on the UI goroutine.
func (a *App) ensureGuildRow(guild store.GuildID) {
	switch guild {
	case backend.DirectMessagesGuildID:
		a.store.UpsertGuild(store.Guild{ID: guild, Name: "Direct Messages"})
	case UngroupedRoomsGuildID:
		a.store.UpsertGuild(store.Guild{ID: guild, Name: "Rooms"})
	default:
		if _, ok := a.store.Guild(guild); !ok {
			a.store.UpsertGuild(store.Guild{ID: guild, Name: "Space"})
		}
	}
}

// roomDisplayName derives a human name for a room lacking an m.room.name, from
// its cached members (best for DMs and small rooms).
func (a *App) roomDisplayName(roomID id.RoomID) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	names := a.memberNames[roomID]
	for uid, n := range names {
		if uid != a.selfID && n != "" {
			return n
		}
	}
	return "Room"
}

func (a *App) fireGuildChange() {
	if a.onGuildChange != nil {
		a.onGuildChange()
	}
	if a.onChange != nil {
		a.onChange()
	}
}

// markReadyOnce fires OnReady the first time a room is materialized, so the UI
// can select an initial channel. Runs on the UI goroutine.
func (a *App) markReadyOnce() {
	a.mu.Lock()
	if a.ready {
		a.mu.Unlock()
		return
	}
	a.ready = true
	a.mu.Unlock()
	if a.onReady != nil {
		a.onReady()
	}
}

func displayOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
