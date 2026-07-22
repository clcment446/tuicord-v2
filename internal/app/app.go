// Package app orchestrates the Discord session, the normalized store, and the TUI runtime.
package app

import (
	clientdiscord "awesomeProject/internal/discord"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"
	"context"
	"encoding/json"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/diamondburned/ningen/v3"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// poster is the slice of tui.App the orchestrator depends on. It exists so the
// orchestration logic can be tested without a real terminal runtime.
type poster interface {
	Post(func())
	// WriteRaw queues raw terminal bytes to be flushed between frames (used for
	// mpv's inline video graphics). Invalidate forces a redraw. ForceRepaint
	// re-emits every cell and graphic (used after mpv painted over the screen).
	WriteRaw([]byte)
	Invalidate()
	ForceRepaint()
}

// EventSink receives client events for out-of-tree consumers (the Lua plugin
// system). It is an optional seam: App calls it via emit only when one is set,
// so this package never depends on the plugin package. Emit must not block —
// implementations are expected to enqueue and return. Payload snowflake fields
// are uint64.
type EventSink interface {
	Emit(name string, data map[string]any)
}

// sender is the slice of the arikawa client used to send messages.
type sender interface {
	SendMessageComplex(discord.ChannelID, api.SendMessageData) (*discord.Message, error)
	EditText(discord.ChannelID, discord.MessageID, string) (*discord.Message, error)
	DeleteMessage(discord.ChannelID, discord.MessageID, api.AuditLogReason) error
	PinMessage(discord.ChannelID, discord.MessageID, api.AuditLogReason) error
	UnpinMessage(discord.ChannelID, discord.MessageID, api.AuditLogReason) error
	CrosspostMessage(discord.ChannelID, discord.MessageID) (*discord.Message, error)
	React(discord.ChannelID, discord.MessageID, discord.APIEmoji) error
}

// threadClient is the slice of the arikawa client used to list and mutate
// threads: active-thread sync on guild open, archived-thread pagination,
// thread creation from a message, and join/leave.
type threadClient interface {
	ActiveThreads(discord.GuildID) (*api.ActiveThreads, error)
	PublicArchivedThreads(discord.ChannelID, discord.Timestamp, uint) (*api.ArchivedThreads, error)
	StartThreadWithMessage(discord.ChannelID, discord.MessageID, api.StartThreadData) (*discord.Channel, error)
	StartThreadWithoutMessage(discord.ChannelID, api.StartThreadData) (*discord.Channel, error)
	JoinThread(discord.ChannelID) error
	LeaveThread(discord.ChannelID) error
}

// forumPoster creates a forum post (a thread with an embedded first message and
// applied tags). arikawa has no typed helper for the user-account forum-create
// payload, so it is posted as raw JSON; the seam keeps it testable.
type forumPoster interface {
	postForumThread(channel store.ChannelID, p forumThreadPayload) (store.ChannelID, error)
}

// historyLoader is the slice of the arikawa client used to load channel
// history.
type historyLoader interface {
	Messages(discord.ChannelID, uint) ([]discord.Message, error)
	MessagesBefore(discord.ChannelID, discord.MessageID, uint) ([]discord.Message, error)
}

type roleLoader interface {
	Roles(discord.GuildID) ([]discord.Role, error)
}

type roleManager interface {
	CreateRole(discord.GuildID, api.CreateRoleData) (*discord.Role, error)
	ModifyRole(discord.GuildID, discord.RoleID, api.ModifyRoleData) (*discord.Role, error)
	DeleteRole(discord.GuildID, discord.RoleID, api.AuditLogReason) error
	MoveRoles(discord.GuildID, api.MoveRolesData) ([]discord.Role, error)
}

type directoryLoader interface {
	Guilds(uint) ([]discord.Guild, error)
	PrivateChannels() ([]discord.Channel, error)
}

type channelLoader interface {
	Channels(discord.GuildID) ([]discord.Channel, error)
}

type channelDetailsLoader interface {
	Channel(discord.ChannelID) (*discord.Channel, error)
}

type memberDetailsLoader interface {
	Member(discord.GuildID, discord.UserID) (*discord.Member, error)
}

type channelManager interface {
	CreateChannel(discord.GuildID, api.CreateChannelData) (*discord.Channel, error)
	ModifyChannel(discord.ChannelID, api.ModifyChannelData) error
	DeleteChannel(discord.ChannelID, api.AuditLogReason) error
	MoveChannels(discord.GuildID, api.MoveChannelsData) error
}

type gifSearcher interface {
	SearchGIFs(string) ([]clientdiscord.GIFResult, error)
}

type restGIFSearcher struct{ client *api.Client }

func (s restGIFSearcher) SearchGIFs(query string) ([]clientdiscord.GIFResult, error) {
	return clientdiscord.SearchGIFs(s.client, query)
}

// DirectMessagesGuildID is the synthetic guild that owns private channels in
// the UI. It avoids overloading guild ID 0, which App uses as "not selected".
const DirectMessagesGuildID store.GuildID = ^store.GuildID(0)

// messageComponentInteractionType is Discord's interaction type for component
// presses (INTERACTION_TYPE 3).
const messageComponentInteractionType = 3

// componentInteraction is the REST payload a user account posts to the
// interactions endpoint when activating a message component. Bots respond to
// interactions; user clients originate them, which is why arikawa has no API
// for this direction. Snowflakes travel as strings per Discord's JSON contract.
type componentInteraction struct {
	Type          int                      `json:"type"`
	Nonce         string                   `json:"nonce"`
	GuildID       string                   `json:"guild_id,omitempty"`
	ChannelID     string                   `json:"channel_id"`
	MessageID     string                   `json:"message_id"`
	ApplicationID string                   `json:"application_id"`
	SessionID     string                   `json:"session_id"`
	MessageFlags  uint64                   `json:"message_flags,omitempty"`
	Data          componentInteractionData `json:"data"`
}

type componentInteractionData struct {
	ComponentType int      `json:"component_type"`
	CustomID      string   `json:"custom_id"`
	Values        []string `json:"values,omitempty"`
}

// componentInteractionPoster is the seam through which component interactions
// reach Discord, sliced narrow so tests can capture the payload.
type componentInteractionPoster interface {
	postComponentInteraction(p componentInteraction) error
}

const applicationCommandInteractionType = 2

// commandInteraction is the user-client payload for a CHAT_INPUT application
// command. It is intentionally separate from componentInteraction: commands
// have no source message and carry the selected command's version.
type commandInteraction struct {
	Type          int                    `json:"type"`
	Nonce         string                 `json:"nonce"`
	GuildID       string                 `json:"guild_id,omitempty"`
	ChannelID     string                 `json:"channel_id"`
	ApplicationID string                 `json:"application_id"`
	SessionID     string                 `json:"session_id"`
	Data          commandInteractionData `json:"data"`
}

type commandInteractionData struct {
	ID                 string                        `json:"id"`
	Name               string                        `json:"name"`
	Type               int                           `json:"type"`
	Version            string                        `json:"version"`
	Options            []commandInteractionOption    `json:"options,omitempty"`
	Attachments        []any                         `json:"attachments"`
	ApplicationCommand interactionApplicationCommand `json:"application_command"`
}

// interactionApplicationCommand restores client-only fields that Arikawa's
// public command model does not retain, notably integration_types.
type interactionApplicationCommand struct {
	Command          ApplicationCommand
	IntegrationTypes []int
	raw              json.RawMessage
}

func (c interactionApplicationCommand) MarshalJSON() ([]byte, error) {
	if len(c.raw) != 0 {
		return append([]byte(nil), c.raw...), nil
	}
	if len(c.Command.raw) != 0 {
		return append([]byte(nil), c.Command.raw...), nil
	}
	encoded, err := json.Marshal(c.Command)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, err
	}
	object["integration_types"] = c.IntegrationTypes
	return json.Marshal(object)
}

type commandInteractionPoster interface {
	postCommandInteraction(p commandInteraction) error
}

const applicationCommandAutocompleteInteractionType = 4

type commandAutocompleteInteraction = commandInteraction

type commandInteractionOption struct {
	Type    int                        `json:"type"`
	Name    string                     `json:"name"`
	Value   any                        `json:"value,omitempty"`
	Focused bool                       `json:"focused,omitempty"`
	Options []commandInteractionOption `json:"options,omitempty"`
}

// CommandChoice is an application-provided autocomplete result. Value retains
// its JSON scalar type because Discord accepts strings, integers, and numbers.
type CommandChoice struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

type commandAutocompleteResponse struct {
	Choices []CommandChoice `json:"choices"`
}

type commandAutocompletePoster interface {
	postCommandAutocomplete(p commandAutocompleteInteraction) ([]CommandChoice, error)
}

// restInteractionPoster posts interactions through the session's REST client.
type restInteractionPoster struct {
	sess *session.Session
}

func (r restInteractionPoster) postComponentInteraction(p componentInteraction) error {
	url := strings.TrimSuffix(api.EndpointInteractions, "/")
	return r.sess.FastRequest("POST", url, httputil.WithJSONBody(p))
}

type restCommandInteractionPoster struct{ sess *session.Session }

func (r restCommandInteractionPoster) postCommandInteraction(p commandInteraction) error {
	url := strings.TrimSuffix(api.EndpointInteractions, "/")
	return r.sess.FastRequest("POST", url, httputil.WithJSONBody(p))
}

type restCommandAutocompletePoster struct{ sess *session.Session }

func (r restCommandAutocompletePoster) postCommandAutocomplete(p commandAutocompleteInteraction) ([]CommandChoice, error) {
	var response commandAutocompleteResponse
	url := strings.TrimSuffix(api.EndpointInteractions, "/")
	err := r.sess.RequestJSON(&response, "POST", url, httputil.WithJSONBody(p))
	return response.Choices, err
}

// App wires the session, store, and UI together and tracks navigation state.
type App struct {
	store               *store.Store
	ui                  poster
	send                sender
	history             historyLoader
	roles               roleLoader
	roleManage          roleManager
	dirs                directoryLoader
	chans               channelLoader
	channelDetail       channelDetailsLoader
	memberDetail        memberDetailsLoader
	channelManage       channelManager
	threads             threadClient
	forum               forumPoster
	interact            componentInteractionPoster
	commandInteract     commandInteractionPoster
	commandAutocomplete commandAutocompletePoster
	commandCatalog      commandCatalogLoader
	gifs                gifSearcher
	// handle is the ningen state: it registers gateway handlers on ningen's
	// forwarded Handler (so ReadState/MemberState are already updated when they
	// fire), opens the gateway, and backs the REST calls that go through the
	// embedded session (e.g. ModifyChannel). ReadState feeds the account badge.
	handle       *ningen.State
	commandMu    sync.Mutex
	commandCache map[CommandContext]commandCacheEntry
	now          func() time.Time

	resourceMu       sync.Mutex
	historyGate      loadGate[store.ChannelID]
	rolesGate        loadGate[store.GuildID]
	guildsGate       singleLoadGate
	channelsGate     loadGate[store.GuildID]
	threadsGate      loadGate[store.GuildID]
	archivedGate     loadGate[store.ChannelID]
	archivedBefore   map[store.ChannelID]discord.Timestamp
	forumMetaPending map[store.ChannelID]uint64

	onReady           func()
	onChange          func()
	onReadStateChange func()
	onGuildChange     func()
	onError           func(error)
	onIncomingMessage func(store.Message)
	events            EventSink

	activeGuild   store.GuildID
	activeChannel store.ChannelID
	selfID        store.UserID
	// stateSnapshot publishes the UI-owned selection and identity to readers on
	// other goroutines (notably synchronous Lua accessors) without posting back
	// to the UI loop, which may not be running during startup or shutdown.
	stateSnapshot atomic.Pointer[StateSnapshot]
	// unreadMu protects derived read-state data updated by gateway goroutines.
	unreadMu       sync.RWMutex
	unreadChannels map[store.GuildID]map[store.ChannelID]UnreadStatus
	guildUnread    map[store.GuildID]UnreadStatus
	// sessionID is the gateway session identifier from READY; Discord requires
	// it on user-originated interaction payloads.
	sessionID string
}

// UnreadStatus is the server-authoritative attention state for a guild or
// channel. Mentions take precedence over ordinary unread messages.
type UnreadStatus uint8

const (
	Read UnreadStatus = iota
	Unread
	Mentioned
)

// New returns an orchestrator over the given ningen state, store, and UI
// runtime. The REST interface slices are backed by the embedded arikawa
// session (ningen does not change the REST surface), while gateway handler
// registration and Connect go through ningen so its caches stay authoritative.
func New(n *ningen.State, st *store.Store, ui *tui.App) *App {
	sess := n.Session
	a := &App{
		store:               st,
		ui:                  ui,
		send:                sess,
		history:             sess,
		roles:               sess,
		roleManage:          sess,
		dirs:                sess,
		chans:               sess,
		channelDetail:       sess,
		memberDetail:        sess,
		channelManage:       sess,
		threads:             sess,
		forum:               restForumPoster{sess: sess},
		interact:            restInteractionPoster{sess: sess},
		commandInteract:     restCommandInteractionPoster{sess: sess},
		commandAutocomplete: restCommandAutocompletePoster{sess: sess},
		commandCatalog:      restCommandCatalogLoader{sess: sess},
		gifs:                restGIFSearcher{client: sess.Client},
		handle:              n,
		commandCache:        make(map[CommandContext]commandCacheEntry),
		now:                 time.Now,
	}
	return a
}

// CommandOption is a validated form value for a chat-input command. Nested
// Options represent a selected subcommand or subcommand group.
type CommandOption struct {
	Name    string
	Type    discord.CommandOptionType
	Value   any
	Focused bool
	Options []CommandOption
}

func (a *App) ensureResourceMaps() {
	a.historyGate.ensure()
	a.rolesGate.ensure()
	a.channelsGate.ensure()
	a.threadsGate.ensure()
	a.archivedGate.ensure()
	if a.archivedBefore == nil {
		a.archivedBefore = map[store.ChannelID]discord.Timestamp{}
	}
	if a.forumMetaPending == nil {
		a.forumMetaPending = map[store.ChannelID]uint64{}
	}
}

// Store returns the underlying state store (read on the UI goroutine).
func (a *App) Store() *store.Store { return a.store }

// Post schedules fn on the UI event loop.
func (a *App) Post(fn func()) {
	if a != nil && a.ui != nil {
		a.ui.Post(fn)
	}
}

// TryPost reports whether the UI event loop accepted fn. Older/test poster
// implementations retain Post-only behavior; the real tui.App rejects work once
// shutdown starts so asynchronous resource owners can clean up immediately.
func (a *App) TryPost(fn func()) bool {
	if a == nil || a.ui == nil || fn == nil {
		return false
	}
	if p, ok := a.ui.(interface{ TryPost(func()) bool }); ok {
		return p.TryPost(fn)
	}
	a.ui.Post(fn)
	return true
}

// WriteRaw queues raw terminal bytes for the UI loop to flush between frames.
func (a *App) WriteRaw(b []byte) {
	if a != nil && a.ui != nil {
		a.ui.WriteRaw(b)
	}
}

// Invalidate forces the UI to redraw on its next loop turn.
func (a *App) Invalidate() {
	if a != nil && a.ui != nil {
		a.ui.Invalidate()
	}
}

// ForceRepaint forces the UI to re-emit every cell and graphic on its next turn.
func (a *App) ForceRepaint() {
	if a != nil && a.ui != nil {
		a.ui.ForceRepaint()
	}
}

// ActiveGuild returns the currently selected guild.
func (a *App) ActiveGuild() store.GuildID { return a.activeGuild }

// ActiveChannel returns the currently selected channel.
func (a *App) ActiveChannel() store.ChannelID { return a.activeChannel }

// SelfID returns the logged-in user's ID once READY has been processed.
func (a *App) SelfID() store.UserID { return a.selfID }

// StateSnapshot is an immutable, concurrently readable view of the small piece
// of UI-owned state exposed to integrations. Call App.Snapshot instead of
// reading ActiveGuild, ActiveChannel, or SelfID from a background goroutine.
type StateSnapshot struct {
	ActiveGuild   store.GuildID
	ActiveChannel store.ChannelID
	SelfID        store.UserID
}

// Snapshot returns the latest published app state without waiting for the UI
// event loop. Before READY/selection it returns zero values.
func (a *App) Snapshot() StateSnapshot {
	if a == nil {
		return StateSnapshot{}
	}
	if snapshot := a.stateSnapshot.Load(); snapshot != nil {
		return *snapshot
	}
	return StateSnapshot{}
}

// publishStateSnapshot must be called on the UI goroutine after changing one of
// the fields represented by StateSnapshot.
func (a *App) publishStateSnapshot() {
	a.stateSnapshot.Store(&StateSnapshot{
		ActiveGuild:   a.activeGuild,
		ActiveChannel: a.activeChannel,
		SelfID:        a.selfID,
	})
}

// SetActive selects the guild/channel the chat view renders, clearing the newly
// active channel's unread badge. Call on the UI goroutine.
func (a *App) SetActive(guild store.GuildID, channel store.ChannelID) {
	a.activeGuild = guild
	a.activeChannel = channel
	a.publishStateSnapshot()
	if channel != 0 {
		a.store.ClearUnread(channel)
	}
	a.emit("channel.switch", map[string]any{
		"guild_id":   uint64(guild),
		"channel_id": uint64(channel),
	})
}

// OnReady registers a callback run (on the UI goroutine) after the READY event
// has populated the store, so the UI can select an initial channel.
func (a *App) OnReady(fn func()) { a.onReady = fn }

// OnChange registers a callback run on the UI goroutine after channel-scoped
// state changes, so the UI can refresh channel rows and unread badges.
func (a *App) OnChange(fn func()) { a.onChange = fn }

// OnReadStateChange registers a callback for read-state-only changes. Keeping
// it separate avoids rebuilding the active channel when only a guild dot moves.
func (a *App) OnReadStateChange(fn func()) { a.onReadStateChange = fn }

// OnGuildChange registers a callback run on the UI goroutine after the guild
// directory changes. Guild lifecycle events also invoke OnChange.
func (a *App) OnGuildChange(fn func()) { a.onGuildChange = fn }

// OnIncomingMessage registers a callback for a newly received remote message.
// It runs on the UI goroutine after the message has been added to the store.
func (a *App) OnIncomingMessage(fn func(store.Message)) { a.onIncomingMessage = fn }

// OnError registers a callback run (on the UI goroutine) when background work
// fails but the client can keep running.
func (a *App) OnError(fn func(error)) { a.onError = fn }

// SetEventSink registers an optional consumer of client events (the plugin
// system). Pass nil to detach.
func (a *App) SetEventSink(sink EventSink) { a.events = sink }

// emit forwards an event to the registered sink, if any. Safe to call with a
// nil receiver or no sink.
func (a *App) emit(name string, data map[string]any) {
	if a == nil || a.events == nil {
		return
	}
	a.events.Emit(name, data)
}

// RegisterHandlers subscribes to the gateway events the client consumes. Each
// handler marshals its store mutation onto the UI goroutine via Post.
func (a *App) RegisterHandlers() {
	a.registerGatewayLifecycleHandlers()
	a.registerGatewayMessageHandlers()
	a.registerGatewayMemberHandlers()
	a.registerGatewayThreadHandlers()
}

// ComponentSubmit describes an activated message component to submit to
// Discord: which message it lives on, which control (CustomID), and any
// selected values for select menus.
type ComponentSubmit struct {
	Message store.Message
	// ComponentType is Discord's numeric component type (2 button, 3 string
	// select, 5-8 entity selects). Zero falls back to button.
	ComponentType int
	CustomID      string
	Values        []string
}

type historyRequestSnapshot struct {
	revision          uint64
	channelGeneration uint64
	gateVersion       uint64
}

type directoryRequestSnapshot struct {
	guilds      map[store.GuildID]uint64
	channels    map[store.ChannelID]uint64
	gateVersion uint64
}

// dmHydrationConcurrency bounds how many DM channel-detail requests run at once
// when filling recipients omitted from a sparse startup payload. A small cap
// keeps the launch fetch well under Discord's rate limits instead of bursting
// one request per DM at the same instant.
const dmHydrationConcurrency = 4

// Connect opens the gateway and blocks until ctx is canceled. Connect is
// promoted from the embedded arikawa session, so it keeps the reconnect loop
// and fatal-close (4004) reporting. Ningen's ReadState/MemberState/etc. are
// populated by ningen's own synchronous handler, which fires on every gateway
// event whether the connection was opened via Connect or ningen.Open.
func (a *App) Connect(ctx context.Context) error {
	return a.handle.Connect(ctx)
}

// Ningen returns the underlying ningen state, exposing ReadState (for the
// account unread badge), MemberState, MutedState, and EmojiState. It is nil
// only in tests that construct App without a handle.
func (a *App) Ningen() *ningen.State {
	if a == nil {
		return nil
	}
	return a.handle
}

// Unread reports this account's aggregate unread state for the multi-account
// selector badge. The mention count is authoritative from ningen's ReadState;
// the unread flag is set when there are mentions or any locally tracked unread
// or attention messages in this account's store. Safe before connect (returns
// zero/false). All reads happen on the UI goroutine, like every store access.
func (a *App) Unread() (unread bool, mentions int) {
	if a == nil {
		return false, 0
	}
	if a.handle != nil && a.handle.ReadState != nil {
		mentions = a.handle.ReadState.TotalMentionCount()
	}
	unread = mentions > 0
	if a.store != nil && (a.store.TotalUnread() > 0 || a.store.TotalPings() > 0) {
		unread = true
	}
	return unread, mentions
}

// ChannelUnread reports a channel's unread state from Discord's read state.
// A local store fallback keeps the sidebar useful before READY has populated
// read states and in tests that do not have a connected gateway.
func (a *App) ChannelUnread(channel store.ChannelID) UnreadStatus {
	status, _ := a.channelUnread(channel)
	return status
}

func (a *App) channelUnread(channel store.ChannelID) (UnreadStatus, bool) {
	if a == nil || a.store == nil || channel == 0 {
		return Read, false
	}
	if status, ok := a.authoritativeChannelUnread(channel); ok {
		return status, true
	}
	if cached, ok := a.cachedChannelUnread(channel); ok {
		return cached, true
	}
	if a.store.Pings(channel) > 0 {
		return Mentioned, false
	}
	if a.store.Unread(channel) > 0 {
		return Unread, false
	}
	return Read, false
}

// authoritativeChannelUnread applies ningen's effective mute and permission
// semantics when Discord has supplied a read position for the channel.
func (a *App) authoritativeChannelUnread(channel store.ChannelID) (UnreadStatus, bool) {
	if a == nil || a.handle == nil || a.handle.ReadState == nil || channel == 0 {
		return Read, false
	}
	id := discord.ChannelID(channel)
	if a.handle.ReadState.ReadState(id) == nil {
		return Read, false
	}
	switch a.handle.ChannelIsUnread(id, ningen.UnreadOpts{}) {
	case ningen.ChannelMentioned:
		return Mentioned, true
	case ningen.ChannelUnread:
		return Unread, true
	default:
		return Read, true
	}
}

// cachedChannelUnread returns event-derived attention state for channels that
// arrived before ningen's cabinet/read-state hydration.
func (a *App) cachedChannelUnread(channel store.ChannelID) (UnreadStatus, bool) {
	if a == nil || a.store == nil || channel == 0 {
		return Read, false
	}
	entry, ok := a.store.Channel(channel)
	if !ok || entry.GuildID == 0 {
		return Read, false
	}
	a.unreadMu.RLock()
	status, ok := a.unreadChannels[entry.GuildID][channel]
	a.unreadMu.RUnlock()
	return status, ok
}

// GuildUnread returns the strongest read state among a guild's channels.
func (a *App) GuildUnread(guild store.GuildID) UnreadStatus {
	if a == nil || guild == 0 {
		return Read
	}
	a.unreadMu.RLock()
	status := a.guildUnread[guild]
	cacheReady := a.unreadChannels != nil
	a.unreadMu.RUnlock()
	// Keep a constant-time local fallback for App values constructed without a
	// ningen handle, and while authoritative state has not announced a local
	// ping yet. Connected sessions otherwise use only the event-driven cache.
	if a.store != nil && a.store.GuildPings(guild) > 0 {
		return Mentioned
	}
	if cacheReady || a.handle != nil {
		return status
	}
	return status
}

// resetReadStateCache replaces the whole derived cache after READY. It scans
// only the already-known store directory; it never issues REST requests.
func (a *App) resetReadStateCache() {
	if a == nil {
		return
	}
	channels := make(map[store.GuildID]map[store.ChannelID]UnreadStatus)
	guilds := make(map[store.GuildID]UnreadStatus)
	if a.store != nil {
		for _, guild := range a.store.Guilds() {
			for _, channel := range a.store.Channels(guild.ID) {
				status, authoritative := a.authoritativeChannelUnread(channel.ID)
				if !authoritative {
					switch {
					case a.store.Pings(channel.ID) > 0:
						status = Mentioned
					case a.store.Unread(channel.ID) > 0:
						status = Unread
					}
				}
				if status == Read {
					// An authoritative read position must also retire the local
					// fallback, otherwise GuildPings can override it forever.
					if authoritative {
						a.store.ClearUnread(channel.ID)
					}
					continue
				}
				if channels[guild.ID] == nil {
					channels[guild.ID] = make(map[store.ChannelID]UnreadStatus)
				}
				channels[guild.ID][channel.ID] = status
				if status > guilds[guild.ID] {
					guilds[guild.ID] = status
				}
			}
		}
	}
	a.unreadMu.Lock()
	a.unreadChannels = channels
	a.guildUnread = guilds
	a.unreadMu.Unlock()
}

// removeCachedReadState drops one deleted channel and recomputes its guild.
// A zero guild searches the small derived cache for the owning guild.
func (a *App) removeCachedReadState(channel store.ChannelID, guild store.GuildID) bool {
	if a == nil || channel == 0 {
		return false
	}
	a.unreadMu.Lock()
	defer a.unreadMu.Unlock()
	if guild == 0 {
		for candidate, channels := range a.unreadChannels {
			if _, ok := channels[channel]; ok {
				guild = candidate
				break
			}
		}
	}
	channels := a.unreadChannels[guild]
	if _, ok := channels[channel]; !ok {
		return false
	}
	delete(channels, channel)
	aggregate := Read
	for _, status := range channels {
		if status > aggregate {
			aggregate = status
		}
	}
	if aggregate == Read {
		delete(a.guildUnread, guild)
	} else {
		a.guildUnread[guild] = aggregate
	}
	return true
}

// removeGuildReadState drops all derived state for a permanently deleted guild.
func (a *App) removeGuildReadState(guild store.GuildID) bool {
	if a == nil || guild == 0 {
		return false
	}
	a.unreadMu.Lock()
	defer a.unreadMu.Unlock()
	_, channels := a.unreadChannels[guild]
	_, aggregate := a.guildUnread[guild]
	delete(a.unreadChannels, guild)
	delete(a.guildUnread, guild)
	return channels || aggregate
}

// cacheReadState records one channel's latest authoritative attention state
// and updates its guild aggregate without scanning the guild's channels.
func (a *App) cacheReadState(channel store.ChannelID, guild store.GuildID, status UnreadStatus) {
	if a == nil || channel == 0 || guild == 0 {
		return
	}
	a.unreadMu.Lock()
	defer a.unreadMu.Unlock()
	if a.unreadChannels == nil {
		a.unreadChannels = make(map[store.GuildID]map[store.ChannelID]UnreadStatus)
	}
	if a.guildUnread == nil {
		a.guildUnread = make(map[store.GuildID]UnreadStatus)
	}
	channels := a.unreadChannels[guild]
	if channels == nil {
		channels = make(map[store.ChannelID]UnreadStatus)
		a.unreadChannels[guild] = channels
	}
	if status == Read {
		delete(channels, channel)
	} else {
		channels[channel] = status
	}
	aggregate := Read
	for _, next := range channels {
		if next > aggregate {
			aggregate = next
		}
	}
	if aggregate == Read {
		delete(a.guildUnread, guild)
	} else {
		a.guildUnread[guild] = aggregate
	}
}

// MarkRead acknowledges a message with Discord and clears the local fallback.
// Read positions never move backwards when the focus bar visits an older row.
func (a *App) MarkRead(channel store.ChannelID, message store.MessageID) {
	if a == nil || a.store == nil || channel == 0 {
		return
	}
	if latest, ok := a.store.LastMsg(channel); ok && latest.ID > message {
		message = latest.ID
	}
	if latest, ok := a.store.Channel(channel); ok && latest.LastMessageID > message {
		message = latest.LastMessageID
	}
	if a.handle != nil {
		id := discord.ChannelID(channel)
		if a.handle.ReadState != nil {
			if state := a.handle.ReadState.ReadState(id); state != nil && store.MessageID(state.LastMessageID) > message {
				message = store.MessageID(state.LastMessageID)
			}
		}
		if latest := store.MessageID(a.handle.LastMessage(id)); latest > message {
			message = latest
		}
	}
	if message == 0 {
		return
	}
	if a.handle != nil && a.handle.ReadState != nil {
		a.handle.ReadState.MarkRead(discord.ChannelID(channel), discord.MessageID(message))
	}
	a.store.ClearUnread(channel)
	if a.onChange != nil {
		a.onChange()
	}
}

// MarkChannelRead acknowledges the newest locally known message in a channel.
func (a *App) MarkChannelRead(channel store.ChannelID) {
	if a == nil || a.store == nil {
		return
	}
	if latest, ok := a.store.LastMsg(channel); ok {
		a.MarkRead(channel, latest.ID)
		return
	}
	if a.handle != nil {
		message := store.MessageID(a.handle.LastMessage(discord.ChannelID(channel)))
		if message != 0 {
			a.MarkRead(channel, message)
		} else {
			a.store.ClearUnread(channel)
		}
		return
	}
	a.store.ClearUnread(channel)
}
