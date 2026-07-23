// Package matrixapp orchestrates a Matrix session into the protocol-neutral
// store and TUI runtime, mirroring the role internal/app plays for Discord. It
// implements backend.Backend, so the UI, account manager, and plugin host drive
// a Matrix account exactly as they drive a Discord one.
//
// Concurrency model: the mautrix sync loop dispatches handlers synchronously on
// a single goroutine; each handler builds store mutations and enqueues them onto
// the one UI goroutine via ui.Post (FIFO), so the store is only ever written
// from the UI goroutine — identical to the Discord orchestrator's discipline.
package matrixapp

import (
	"sync"
	"sync/atomic"

	"maunium.net/go/mautrix/id"

	"awesomeProject/internal/backend"
	"awesomeProject/internal/matrix"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"
)

// UngroupedRoomsGuildID is the synthetic guild that owns joined rooms not
// claimed by any space. It sits alongside backend.DirectMessagesGuildID.
const UngroupedRoomsGuildID store.GuildID = ^store.GuildID(1)

// poster is the slice of tui.App the orchestrator posts UI work through.
type poster interface {
	Post(func())
	TryPost(func()) bool
	WriteRaw([]byte)
	Invalidate()
	ForceRepaint()
}

// roomInfo is the accumulated per-room knowledge used to place a room in the
// sidebar and route its events.
type roomInfo struct {
	channelID store.ChannelID
	name      string
	topic     string
	avatar    string
	isSpace   bool
	isDM      bool
	// children are ordered child room mxids when this room is a space.
	children []string
	// prevBatch is the pagination token for backfilling older history.
	prevBatch string
}

// reactionRef locates the target of a reaction event so a later redaction can
// remove it.
type reactionRef struct {
	channel store.ChannelID
	message store.MessageID
	key     string
}

// App is the Matrix orchestrator. It satisfies backend.Backend.
type App struct {
	client *matrix.Client
	store  *store.Store
	ui     poster
	ids    *idMap

	selfID id.UserID
	self   store.Member

	mu               sync.Mutex
	activeGuild      store.GuildID
	activeChannel    store.ChannelID
	rooms            map[id.RoomID]*roomInfo
	roomByChannel    map[store.ChannelID]id.RoomID
	reactions        map[id.EventID]reactionRef // reaction event -> target (for redaction routing)
	reactionOrder    []id.EventID               // FIFO of reaction event ids, bounds the map
	directRooms      map[id.RoomID]bool
	childToSpace     map[id.RoomID]id.RoomID            // child room -> parent space room
	memberNames      map[id.RoomID]map[id.UserID]string // per-room display names
	channelUnread    map[store.ChannelID]backend.UnreadStatus
	channelHighlight map[store.ChannelID]int
	guildUnread      map[store.GuildID]backend.UnreadStatus
	mentionTotal     int
	ready            bool

	stateSnapshot atomic.Pointer[backend.StateSnapshot]

	onReady           func()
	onChange          func()
	onGuildChange     func()
	onReadStateChange func()
	onIncoming        func(store.Message)
	onError           func(error)
	events            backend.EventSink

	authorizer *mediaAuthorizer
}

var _ backend.Backend = (*App)(nil)

// New builds a Matrix orchestrator over a connected client, store, and UI.
// idmapPath persists the room/user id interning table across restarts.
func New(client *matrix.Client, st *store.Store, ui *tui.App, idmapPath string) *App {
	a := &App{
		client:           client,
		store:            st,
		ui:               ui,
		ids:              newIDMap(idmapPath),
		selfID:           client.UserID(),
		rooms:            map[id.RoomID]*roomInfo{},
		roomByChannel:    map[store.ChannelID]id.RoomID{},
		reactions:        map[id.EventID]reactionRef{},
		directRooms:      map[id.RoomID]bool{},
		childToSpace:     map[id.RoomID]id.RoomID{},
		memberNames:      map[id.RoomID]map[id.UserID]string{},
		channelUnread:    map[store.ChannelID]backend.UnreadStatus{},
		channelHighlight: map[store.ChannelID]int{},
		guildUnread:      map[store.GuildID]backend.UnreadStatus{},
	}
	a.self = store.Member{ID: store.UserID(a.ids.intern(string(a.selfID)))}
	a.authorizer = newMediaAuthorizer(a)
	return a
}

// Store returns the normalized store.
func (a *App) Store() *store.Store { return a.store }

// --- poster passthrough -----------------------------------------------------

func (a *App) Post(fn func())         { a.ui.Post(fn) }
func (a *App) TryPost(fn func()) bool { return a.ui.TryPost(fn) }
func (a *App) WriteRaw(b []byte)      { a.ui.WriteRaw(b) }
func (a *App) Invalidate()            { a.ui.Invalidate() }
func (a *App) ForceRepaint()          { a.ui.ForceRepaint() }

// --- identity & selection ---------------------------------------------------

func (a *App) ActiveGuild() store.GuildID     { return a.activeGuild }
func (a *App) ActiveChannel() store.ChannelID { return a.activeChannel }
func (a *App) SelfID() store.UserID           { return a.self.ID }

func (a *App) Self() (store.Member, bool) {
	if a.self.ID == 0 {
		return store.Member{}, false
	}
	return a.self, true
}

func (a *App) Snapshot() backend.StateSnapshot {
	if s := a.stateSnapshot.Load(); s != nil {
		return *s
	}
	return backend.StateSnapshot{}
}

func (a *App) publishSnapshot() {
	a.stateSnapshot.Store(&backend.StateSnapshot{
		ActiveGuild:   a.activeGuild,
		ActiveChannel: a.activeChannel,
		SelfID:        a.self.ID,
	})
}

// SetActive selects the room the chat view renders and clears its unread badge.
// Marking the room read is done separately by MarkChannelRead when appropriate.
func (a *App) SetActive(guild store.GuildID, channel store.ChannelID) {
	a.activeGuild = guild
	a.activeChannel = channel
	a.publishSnapshot()
	if channel != 0 {
		a.store.ClearUnread(channel)
		a.mu.Lock()
		delete(a.channelUnread, channel)
		delete(a.channelHighlight, channel)
		a.recomputeAggregatesLocked()
		a.mu.Unlock()
	}
	a.emit("channel.switch", map[string]any{
		"guild_id":   uint64(guild),
		"channel_id": uint64(channel),
	})
}

// --- callbacks --------------------------------------------------------------

func (a *App) OnReady(fn func())                        { a.onReady = fn }
func (a *App) OnChange(fn func())                       { a.onChange = fn }
func (a *App) OnGuildChange(fn func())                  { a.onGuildChange = fn }
func (a *App) OnReadStateChange(fn func())              { a.onReadStateChange = fn }
func (a *App) OnIncomingMessage(fn func(store.Message)) { a.onIncoming = fn }
func (a *App) OnError(fn func(error))                   { a.onError = fn }
func (a *App) SetEventSink(sink backend.EventSink)      { a.events = sink }

func (a *App) emit(name string, data map[string]any) {
	if a.events != nil {
		a.events.Emit(name, data)
	}
}

func (a *App) reportError(err error) {
	if err == nil {
		return
	}
	a.ui.Post(func() {
		if a.onError != nil {
			a.onError(err)
		}
	})
}

// --- read state -------------------------------------------------------------

func (a *App) Unread() (bool, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	unread := a.mentionTotal > 0
	if !unread {
		for _, s := range a.channelUnread {
			if s != backend.Read {
				unread = true
				break
			}
		}
	}
	return unread, a.mentionTotal
}

func (a *App) ChannelUnread(channel store.ChannelID) backend.UnreadStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.channelUnread[channel]
}

func (a *App) GuildUnread(guild store.GuildID) backend.UnreadStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.guildUnread[guild]
}

// --- Discord-only surface: no-ops on Matrix ---------------------------------

func (a *App) SendSticker(id uint64)                                                      {}
func (a *App) LoadRoles(guild store.GuildID)                                              {}
func (a *App) LoadForumMetadata(channel store.ChannelID)                                  {}
func (a *App) LoadArchivedThreads(channel store.ChannelID)                                {}
func (a *App) Publish(channel store.ChannelID, message store.MessageID)                   {}
func (a *App) CreateForumPost(forum store.ChannelID, title, body string, tagIDs []uint64) {}
func (a *App) CreateRole(guild store.GuildID, name string)                                {}
func (a *App) RenameRole(guild store.GuildID, role store.RoleID, name string)             {}
func (a *App) SetRoleColor(guild store.GuildID, role store.RoleID, color uint32)          {}
func (a *App) SetRoleHoist(guild store.GuildID, role store.RoleID, value bool)            {}
func (a *App) SetRoleMentionable(guild store.GuildID, role store.RoleID, v bool)          {}
func (a *App) DeleteRole(guild store.GuildID, role store.RoleID)                          {}
func (a *App) MoveRole(guild store.GuildID, role store.RoleID, position int)              {}
func (a *App) CreateTextChannel(guild store.GuildID, name string)                         {}
func (a *App) RenameChannel(id store.ChannelID, name string)                              {}
func (a *App) DeleteChannel(id store.ChannelID)                                           {}
func (a *App) MoveChannel(guild store.GuildID, id store.ChannelID, position int)          {}
func (a *App) JoinThread(thread store.ChannelID)                                          {}
func (a *App) LeaveThread(thread store.ChannelID)                                         {}
func (a *App) SetThreadArchived(thread store.ChannelID, archived bool)                    {}

// EnsureMemberDetail is best-effort on Matrix: lazy-loaded members are already
// hydrated from sync state, so this immediately signals completion.
func (a *App) EnsureMemberDetail(guild store.GuildID, user store.UserID, done func()) {
	if done != nil {
		a.ui.Post(done)
	}
}

// LoadActiveThreads is handled in threads.go.

// Connect / RegisterHandlers / the sync loop live in sync.go.
// Messaging, history, and receipts live in actions.go.
