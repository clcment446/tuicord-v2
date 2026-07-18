// Package app orchestrates the Discord session, the normalized store, and the
// TUI runtime.
//
// It is the single bridge between two worlds: Discord gateway events arrive on
// network goroutines, while the store and widgets live on the UI goroutine. All
// gateway handlers therefore funnel their work through tui.App.Post, which runs
// the closure on the UI goroutine. That keeps the store lock-free (see the
// store package's concurrency note).
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	clientdiscord "awesomeProject/internal/discord"
	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
	"github.com/diamondburned/arikawa/v3/utils/sendpart"
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
	channelManage       channelManager
	threads             threadClient
	forum               forumPoster
	interact            componentInteractionPoster
	commandInteract     commandInteractionPoster
	commandAutocomplete commandAutocompletePoster
	commandCatalog      commandCatalogLoader
	gifs                gifSearcher
	handle              *session.Session
	commandMu           sync.Mutex
	commandCache        map[CommandContext]commandCacheEntry
	now                 func() time.Time

	resourceMu       sync.Mutex
	historyGate      loadGate[store.ChannelID]
	rolesGate        loadGate[store.GuildID]
	guildsGate       singleLoadGate
	channelsGate     loadGate[store.GuildID]
	threadsGate      loadGate[store.GuildID]
	archivedGate     loadGate[store.ChannelID]
	archivedBefore   map[store.ChannelID]discord.Timestamp
	forumMetaPending map[store.ChannelID]struct{}

	onReady           func()
	onChange          func()
	onError           func(error)
	onIncomingMessage func(store.Message)
	events            EventSink

	activeGuild   store.GuildID
	activeChannel store.ChannelID
	selfID        store.UserID
	// sessionID is the gateway session identifier from READY; Discord requires
	// it on user-originated interaction payloads.
	sessionID string
}

// New returns an orchestrator over the given session, store, and UI runtime.
func New(sess *session.Session, st *store.Store, ui *tui.App) *App {
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
		channelManage:       sess,
		threads:             sess,
		forum:               restForumPoster{sess: sess},
		interact:            restInteractionPoster{sess: sess},
		commandInteract:     restCommandInteractionPoster{sess: sess},
		commandAutocomplete: restCommandAutocompletePoster{sess: sess},
		commandCatalog:      restCommandCatalogLoader{sess: sess},
		gifs:                restGIFSearcher{client: sess.Client},
		handle:              sess,
		commandCache:        make(map[CommandContext]commandCacheEntry),
		now:                 time.Now,
	}
	return a
}

// SubmitCommand dispatches a chat-input command in the active channel. The UI
// supplies typed, validated option values; a focused option is only valid for
// the separate autocomplete interaction.
func (a *App) SubmitCommand(command ApplicationCommand, options []CommandOption) {
	if a == nil || a.commandInteract == nil || command.ID == 0 || command.AppID == 0 || command.Name == "" || a.activeChannel == 0 {
		return
	}
	if command.Type != 0 && command.Type != discord.ChatInputCommand {
		return
	}
	wireOptions, focused := commandOptionsToWire(options)
	if focused != 0 {
		a.reportError(fmt.Errorf("slash command %q cannot submit a focused autocomplete option", command.Name))
		return
	}
	payload := commandInteraction{
		Type:          applicationCommandInteractionType,
		Nonce:         newInteractionNonce(),
		ChannelID:     strconv.FormatUint(uint64(a.activeChannel), 10),
		ApplicationID: strconv.FormatUint(uint64(command.AppID), 10),
		SessionID:     a.sessionID,
		Data: commandInteractionData{
			ID:                 strconv.FormatUint(uint64(command.ID), 10),
			Name:               command.Name,
			Type:               int(discord.ChatInputCommand),
			Version:            strconv.FormatUint(uint64(command.Version), 10),
			Options:            wireOptions,
			Attachments:        []any{},
			ApplicationCommand: interactionApplicationCommand{Command: command, IntegrationTypes: commandIntegrationTypes(a.activeGuild)},
		},
	}
	if a.activeGuild != 0 && a.activeGuild != DirectMessagesGuildID {
		payload.GuildID = strconv.FormatUint(uint64(a.activeGuild), 10)
	}
	go func() {
		if err := a.commandInteract.postCommandInteraction(payload); err != nil {
			a.ui.Post(func() { a.reportError(err) })
		}
	}()
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

// AutocompleteCommand asks Discord's application for suggestions for one
// focused command option. The caller is always completed on the UI goroutine.
func (a *App) AutocompleteCommand(command ApplicationCommand, options []CommandOption, done func([]CommandChoice, error)) {
	if a == nil || a.commandAutocomplete == nil || done == nil || command.ID == 0 || command.AppID == 0 || command.Name == "" || a.activeChannel == 0 {
		return
	}
	wireOptions, focused := commandOptionsToWire(options)
	if focused != 1 {
		a.ui.Post(func() { done(nil, fmt.Errorf("autocomplete requires exactly one focused option")) })
		return
	}
	payload := commandAutocompleteInteraction(commandInteraction{
		Type:          applicationCommandAutocompleteInteractionType,
		Nonce:         newInteractionNonce(),
		ChannelID:     strconv.FormatUint(uint64(a.activeChannel), 10),
		ApplicationID: strconv.FormatUint(uint64(command.AppID), 10),
		SessionID:     a.sessionID,
		Data: commandInteractionData{
			ID:                 strconv.FormatUint(uint64(command.ID), 10),
			Name:               command.Name,
			Type:               int(discord.ChatInputCommand),
			Version:            strconv.FormatUint(uint64(command.Version), 10),
			Options:            wireOptions,
			Attachments:        []any{},
			ApplicationCommand: interactionApplicationCommand{Command: command, IntegrationTypes: commandIntegrationTypes(a.activeGuild)},
		},
	})
	if a.activeGuild != 0 && a.activeGuild != DirectMessagesGuildID {
		payload.GuildID = strconv.FormatUint(uint64(a.activeGuild), 10)
	}
	go func() {
		choices, err := a.commandAutocomplete.postCommandAutocomplete(payload)
		a.ui.Post(func() { done(append([]CommandChoice(nil), choices...), err) })
	}()
}

func commandIntegrationTypes(guild store.GuildID) []int {
	if guild == 0 || guild == DirectMessagesGuildID {
		return []int{1}
	}
	return []int{0}
}

func commandOptionsToWire(options []CommandOption) ([]commandInteractionOption, int) {
	wire := make([]commandInteractionOption, 0, len(options))
	focused := 0
	for _, option := range options {
		nested, nestedFocused := commandOptionsToWire(option.Options)
		if option.Focused {
			focused++
		}
		focused += nestedFocused
		wire = append(wire, commandInteractionOption{
			Type:    int(option.Type),
			Name:    option.Name,
			Value:   option.Value,
			Focused: option.Focused,
			Options: nested,
		})
	}
	return wire, focused
}

// SearchGIFs searches Discord's Tenor proxy away from the UI thread and posts
// completion back onto the UI event loop.
func (a *App) SearchGIFs(query string, done func([]clientdiscord.GIFResult, error)) {
	query = strings.TrimSpace(query)
	if a == nil || a.gifs == nil || query == "" || done == nil {
		return
	}
	go func() {
		results, err := a.gifs.SearchGIFs(query)
		a.ui.Post(func() {
			if err != nil && a.onError != nil {
				a.onError(err)
			}
			done(results, err)
		})
	}()
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
		a.forumMetaPending = map[store.ChannelID]struct{}{}
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

// SetActive selects the guild/channel the chat view renders, clearing the newly
// active channel's unread badge. Call on the UI goroutine.
func (a *App) SetActive(guild store.GuildID, channel store.ChannelID) {
	a.activeGuild = guild
	a.activeChannel = channel
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

// OnChange registers a callback run (on the UI goroutine) after an incoming
// message updates the store, so the UI can refresh unread badges.
func (a *App) OnChange(fn func()) { a.onChange = fn }

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
	joined := make(map[store.ChannelID]bool, len(e.Members))
	for _, m := range e.Members {
		joined[store.ChannelID(m.ID)] = true
	}
	a.ui.Post(func() {
		for _, t := range threads {
			if t.Thread != nil && joined[t.ID] {
				t.Thread.Joined = true
			}
			a.store.UpsertChannel(t)
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
	added := false
	for _, m := range e.AddedMembers {
		if store.UserID(m.UserID) == a.selfID && a.selfID != 0 {
			added = true
		}
	}
	removed := false
	for _, uid := range e.RemovedMemberIDs {
		if store.UserID(uid) == a.selfID && a.selfID != 0 {
			removed = true
		}
	}
	if !added && !removed {
		return
	}
	a.ui.Post(func() {
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
	a.ui.Post(func() {
		a.selfID = selfID
		a.sessionID = sessionID
		a.store.SetNitro(hasNitro)
		a.store.SetGuildFolders(folders)
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
		a.markRolesLoaded(store.GuildID(guild.ID))
		if len(guild.Channels) > 0 {
			a.markChannelsLoaded(store.GuildID(guild.ID))
		}
	})
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
		if appended {
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
		a.emit("message.create", map[string]any{
			"id":         uint64(msg.ID),
			"channel_id": uint64(msg.ChannelID),
			"guild_id":   uint64(e.GuildID),
			"author_id":  uint64(msg.AuthorID),
			"author":     msg.Author,
			"content":    msg.Content,
			"bot":        e.Author.Bot,
		})
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
	a.ui.Post(func() {
		a.store.UpdateMessage(channel, id, func(m *store.Message) {
			m.Content = patch.Content
			m.Flags = patch.Flags
			m.Attachments = patch.Attachments
			m.Embeds = patch.Embeds
			m.Stickers = patch.Stickers
			m.Components = patch.Components
			m.ComponentTree = patch.ComponentTree
			m.Pinned = patch.Pinned
		})
		if a.onChange != nil {
			a.onChange()
		}
		a.emit("message.update", map[string]any{
			"id":         uint64(id),
			"channel_id": uint64(channel),
			"guild_id":   uint64(e.GuildID),
			"author_id":  uint64(patch.AuthorID),
			"content":    patch.Content,
		})
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
	react := store.Reaction{
		EmojiName: e.Emoji.Name,
		EmojiID:   uint64(e.Emoji.ID),
		Animated:  e.Emoji.Animated,
		Count:     1,
		Me:        store.UserID(e.UserID) == a.selfID && a.selfID != 0,
	}
	userID := uint64(e.UserID)
	emoji := e.Emoji.Name
	a.ui.Post(func() {
		a.store.AddReaction(channel, id, react)
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
	me := store.UserID(e.UserID) == a.selfID && a.selfID != 0
	userID := uint64(e.UserID)
	a.ui.Post(func() {
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

// Send posts content to the active channel with optimistic local echo.
//
// The message appears immediately as pending. The REST call runs on a new
// goroutine; on failure the message is marked failed (rendered in the error
// style). On success the reconciliation happens when the gateway echoes the
// message back (matched by nonce), so no duplicate appears.
func (a *App) Send(content string) {
	a.SendFiles(content, nil, nil, nil)
}

// SendFiles posts message content and/or uploaded files to the active channel
// with an optimistic local echo. cleanup runs after the multipart request has
// completed (whether it succeeds or fails), and also runs immediately when a
// send cannot be started. It is intended for closing opened files and removing
// managed temporary clipboard uploads.
func (a *App) SendFiles(content string, files []sendpart.File, optimistic []store.Attachment, cleanup func()) {
	if a == nil || (strings.TrimSpace(content) == "" && len(files) == 0) || a.activeChannel == 0 {
		if cleanup != nil {
			cleanup()
		}
		return
	}

	channel := a.activeChannel
	nonce := newNonce()
	fileCopy := append([]sendpart.File(nil), files...)
	attachmentCopy := append([]store.Attachment(nil), optimistic...)
	a.store.AppendMessage(store.Message{
		ChannelID:   channel,
		Author:      "you",
		Content:     content,
		Nonce:       nonce,
		Pending:     true,
		Attachments: attachmentCopy,
	})

	go func() {
		if cleanup != nil {
			defer cleanup()
		}
		a.deliver(channel, api.SendMessageData{Content: content, Nonce: nonce, Files: fileCopy}, nonce)
	}()
}

// SendSticker posts a native Discord sticker to the active channel.
func (a *App) SendSticker(id uint64) {
	if id == 0 || a.activeChannel == 0 {
		return
	}
	channel := a.activeChannel
	nonce := newNonce()
	a.store.AppendMessage(store.Message{ChannelID: channel, Author: "you", Nonce: nonce, Pending: true})
	go a.deliver(channel, api.SendMessageData{
		Nonce: nonce, StickerIDs: []discord.StickerID{discord.StickerID(id)},
	}, nonce)
}

// Reply sends content as a Discord inline reply to message.
func (a *App) Reply(content string, message store.Message, mention bool) {
	if strings.TrimSpace(content) == "" || message.ChannelID == 0 || message.ID == 0 {
		return
	}
	nonce := newNonce()
	a.store.AppendMessage(store.Message{
		ChannelID: message.ChannelID,
		Author:    "you",
		Content:   content,
		Nonce:     nonce,
		Pending:   true,
	})
	data := api.SendMessageData{
		Content: content,
		Nonce:   nonce,
		Reference: &discord.MessageReference{
			MessageID: discord.MessageID(message.ID),
		},
	}
	if !mention {
		data.AllowedMentions = &api.AllowedMentions{
			Parse:       []api.AllowedMentionType{api.AllowRoleMention, api.AllowUserMention, api.AllowEveryoneMention},
			RepliedUser: option.False,
		}
	}
	go a.deliver(message.ChannelID, data, nonce)
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

// SubmitComponent posts a component interaction to Discord on a background
// goroutine. The component is marked pending immediately; on completion it
// flips to success or error, and failures are also reported via OnError. The
// bot's actual reaction (message edit, reply) arrives through the gateway.
func (a *App) SubmitComponent(sub ComponentSubmit) {
	if a == nil || a.interact == nil || sub.CustomID == "" || sub.Message.ID == 0 {
		return
	}
	msg := sub.Message
	appID := msg.ApplicationID
	if appID == 0 {
		appID = uint64(msg.AuthorID)
	}
	componentType := sub.ComponentType
	if componentType == 0 {
		componentType = 2
	}
	payload := componentInteraction{
		Type:          messageComponentInteractionType,
		Nonce:         newNonce(),
		ChannelID:     strconv.FormatUint(uint64(msg.ChannelID), 10),
		MessageID:     strconv.FormatUint(uint64(msg.ID), 10),
		ApplicationID: strconv.FormatUint(appID, 10),
		SessionID:     a.sessionID,
		MessageFlags:  msg.Flags,
		Data: componentInteractionData{
			ComponentType: componentType,
			CustomID:      sub.CustomID,
			Values:        sub.Values,
		},
	}
	if a.activeGuild != 0 && a.activeGuild != DirectMessagesGuildID {
		payload.GuildID = strconv.FormatUint(uint64(a.activeGuild), 10)
	}
	a.store.SetComponentState(msg.ChannelID, msg.ID, sub.CustomID, store.ComponentStatePending)
	if a.onChange != nil {
		a.onChange()
	}
	go func() {
		err := a.interact.postComponentInteraction(payload)
		a.ui.Post(func() {
			state := store.ComponentStateSuccess
			if err != nil {
				state = store.ComponentStateError
			}
			a.store.SetComponentState(msg.ChannelID, msg.ID, sub.CustomID, state)
			if err != nil && a.onError != nil {
				a.onError(err)
			}
			if a.onChange != nil {
				a.onChange()
			}
		})
	}()
}

// SendToChannel posts content to an explicit channel with an optimistic local
// echo, mirroring Send but without requiring the channel to be active. It is
// the seam plugins use for tuicord.send_to. Call on the UI goroutine.
func (a *App) SendToChannel(channel store.ChannelID, content string) {
	if a == nil || channel == 0 || strings.TrimSpace(content) == "" {
		return
	}
	nonce := newNonce()
	a.store.AppendMessage(store.Message{
		ChannelID: channel,
		Author:    "you",
		Content:   content,
		Nonce:     nonce,
		Pending:   true,
	})
	go a.deliver(channel, api.SendMessageData{Content: content, Nonce: nonce}, nonce)
}

func (a *App) deliver(channel store.ChannelID, data api.SendMessageData, nonce string) {
	_, err := a.send.SendMessageComplex(discord.ChannelID(channel), data)
	if err != nil {
		a.ui.Post(func() {
			a.store.MarkFailed(channel, nonce)
			if a.onError != nil {
				a.onError(err)
			}
		})
	}
}

// EditMessage patches a message's content. The visible message updates after
// Discord echoes MESSAGE_UPDATE; failures are reported via OnError.
func (a *App) EditMessage(channel store.ChannelID, id store.MessageID, content string) {
	if channel == 0 || id == 0 {
		return
	}
	a.runInBackground(func() error {
		_, err := a.send.EditText(discord.ChannelID(channel), discord.MessageID(id), content)
		return err
	})
}

// DeleteMessage deletes a message. Local removal waits for MESSAGE_DELETE.
func (a *App) DeleteMessage(channel store.ChannelID, id store.MessageID) {
	if channel == 0 || id == 0 {
		return
	}
	a.runInBackground(func() error {
		return a.send.DeleteMessage(discord.ChannelID(channel), discord.MessageID(id), "")
	})
}

// AddReaction applies the current user's reaction and lets the gateway update
// the local reaction count.
func (a *App) AddReaction(channel store.ChannelID, id store.MessageID, emoji string) {
	if channel == 0 || id == 0 || emoji == "" {
		return
	}
	a.runInBackground(func() error {
		return a.send.React(discord.ChannelID(channel), discord.MessageID(id), discord.APIEmoji(emoji))
	})
}

// SetPinned pins or unpins a message. Discord's pin event omits the message ID,
// so the cached flag is patched after the REST call succeeds.
func (a *App) SetPinned(channel store.ChannelID, id store.MessageID, pinned bool) {
	if channel == 0 || id == 0 {
		return
	}
	go func() {
		var err error
		if pinned {
			err = a.send.PinMessage(discord.ChannelID(channel), discord.MessageID(id), "")
		} else {
			err = a.send.UnpinMessage(discord.ChannelID(channel), discord.MessageID(id), "")
		}
		if err != nil {
			a.reportError(err)
			return
		}
		a.ui.Post(func() {
			a.store.SetMessagePinned(channel, id, pinned)
			if a.onChange != nil {
				a.onChange()
			}
		})
	}()
}

func (a *App) reportError(err error) {
	if err == nil {
		return
	}
	a.ui.Post(func() {
		if a.onError != nil {
			a.onError(err)
		}
		a.emit("error", map[string]any{"message": err.Error()})
	})
}

// LoadHistory fetches recent messages for channel and replaces the local
// history. The REST API returns latest-first; the store keeps oldest-first.
func (a *App) LoadHistory(channel store.ChannelID, limit uint) {
	if a == nil || a.history == nil || channel == 0 {
		return
	}
	if !a.beginHistoryLoad(channel) {
		return
	}
	go a.loadHistory(channel, limit)
}

func (a *App) loadHistory(channel store.ChannelID, limit uint) {
	messages, err := a.history.Messages(discord.ChannelID(channel), limit)
	if err != nil {
		a.finishHistoryLoad(channel, false)
		a.ui.Post(func() {
			if a.onError != nil {
				a.onError(err)
			}
		})
		return
	}
	converted := make([]store.Message, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		converted = append(converted, convertMessage(messages[i]))
	}
	a.ui.Post(func() {
		if ch, ok := a.store.Channel(channel); ok && ch.GuildID != 0 && ch.GuildID != DirectMessagesGuildID {
			guild := discord.GuildID(ch.GuildID)
			for _, message := range messages {
				a.store.RememberMemberIdentity(ch.GuildID, convertMember(discord.Member{User: message.Author}, guild))
			}
		}
		a.store.SetMessages(channel, converted)
		a.finishHistoryLoad(channel, true)
		if limit == 0 || len(messages) < int(limit) {
			a.markHistoryExhausted(channel)
		}
		if a.onChange != nil {
			a.onChange()
		}
	})
}

// LoadOlderHistory fetches the next page before the oldest cached message.
// Calls made while a page is in flight or when Discord has no older messages
// are ignored.
func (a *App) LoadOlderHistory(channel store.ChannelID) {
	if a == nil || a.history == nil || channel == 0 || !a.beginOlderHistoryLoad(channel) {
		return
	}
	messages := a.store.Messages(channel)
	if len(messages) == 0 {
		a.finishOlderHistory(channel, true)
		return
	}
	before := messages[0].ID
	if before == 0 {
		a.finishOlderHistory(channel, true)
		return
	}
	go a.loadOlderHistory(channel, discord.MessageID(before), 50)
}

func (a *App) loadOlderHistory(channel store.ChannelID, before discord.MessageID, limit uint) {
	messages, err := a.history.MessagesBefore(discord.ChannelID(channel), before, limit)
	if err != nil {
		a.finishOlderHistory(channel, false)
		a.reportError(err)
		return
	}
	converted := make([]store.Message, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		converted = append(converted, convertMessage(messages[i]))
	}
	a.ui.Post(func() {
		if ch, ok := a.store.Channel(channel); ok && ch.GuildID != 0 && ch.GuildID != DirectMessagesGuildID {
			guild := discord.GuildID(ch.GuildID)
			for _, message := range messages {
				a.store.RememberMemberIdentity(ch.GuildID, convertMember(discord.Member{User: message.Author}, guild))
			}
		}
		a.store.PrependMessages(channel, converted)
		a.finishOlderHistory(channel, len(messages) < int(limit))
		if len(converted) > 0 && a.onChange != nil {
			a.onChange()
		}
	})
}

// LoadRoles fetches role definitions for a guild once per session. READY and
// GUILD_CREATE usually include roles, so this is primarily a guarded fallback.
func (a *App) LoadRoles(guild store.GuildID) {
	if a == nil || a.roles == nil || guild == 0 || guild == DirectMessagesGuildID {
		return
	}
	if !a.beginRoleLoad(guild) {
		return
	}
	go a.loadRoles(guild)
}

// LoadGuilds fetches the cheap directory data needed to render the server and
// DM lists. It is guarded so startup or reconnect paths do not hammer REST.
func (a *App) LoadGuilds(limit uint) {
	if a == nil || a.dirs == nil {
		return
	}
	if !a.beginGuildLoad() {
		return
	}
	go a.loadGuilds(limit)
}

func (a *App) loadGuilds(limit uint) {
	guilds, guildErr := a.dirs.Guilds(limit)
	privateChannels, dmErr := a.dirs.PrivateChannels()
	if guildErr != nil && dmErr != nil {
		a.finishGuildLoad(false)
		a.ui.Post(func() {
			if a.onError != nil {
				a.onError(guildErr)
			}
		})
		return
	}
	privateChannels = a.hydratePrivateChannels(privateChannels)
	a.ui.Post(func() {
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

// hydratePrivateChannels fills recipient data omitted by some user-session
// startup responses. Each missing DM is fetched concurrently and the enriched
// result is cached in the store with the rest of the directory.
func (a *App) hydratePrivateChannels(channels []discord.Channel) []discord.Channel {
	if a == nil || a.channelDetail == nil {
		return channels
	}
	var wg sync.WaitGroup
	for i := range channels {
		if channels[i].Name != "" || len(channels[i].DMRecipients) > 0 {
			continue
		}
		id := channels[i].ID
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
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
	if !a.beginChannelLoad(guild) {
		return
	}
	go a.loadChannels(guild)
}

// LoadForumMetadata refreshes a forum channel from Discord's channel endpoint.
// Gateway and guild-directory channel objects are not guaranteed to include
// available_tags, while the channel endpoint does.
func (a *App) LoadForumMetadata(channel store.ChannelID) {
	if a == nil || a.channelDetail == nil || channel == 0 {
		return
	}
	a.resourceMu.Lock()
	a.ensureResourceMaps()
	if _, ok := a.forumMetaPending[channel]; ok {
		a.resourceMu.Unlock()
		return
	}
	a.forumMetaPending[channel] = struct{}{}
	a.resourceMu.Unlock()
	go func() {
		c, err := a.channelDetail.Channel(discord.ChannelID(channel))
		a.ui.Post(func() {
			a.resourceMu.Lock()
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
	channels, err := a.chans.Channels(discord.GuildID(guild))
	if err != nil {
		a.finishChannelLoad(guild, false)
		a.ui.Post(func() {
			if a.onError != nil {
				a.onError(err)
			}
		})
		return
	}
	a.ui.Post(func() {
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
	roles, err := a.roles.Roles(discord.GuildID(guild))
	if err != nil {
		a.finishRoleLoad(guild, false)
		a.ui.Post(func() {
			if a.onError != nil {
				a.onError(err)
			}
		})
		return
	}
	a.ui.Post(func() {
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

func (a *App) beginGuildLoad() bool {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	return a.guildsGate.begin()
}

func (a *App) finishGuildLoad(ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.guildsGate.finish(ok)
}

func (a *App) beginHistoryLoad(channel store.ChannelID) bool {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.historyGate.begin(channel)
}

func (a *App) finishHistoryLoad(channel store.ChannelID, ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.historyGate.finish(channel, ok)
}

func (a *App) beginOlderHistoryLoad(channel store.ChannelID) bool {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.historyGate.beginOlder(channel)
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

func (a *App) beginRoleLoad(guild store.GuildID) bool {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.rolesGate.begin(guild)
}

func (a *App) finishRoleLoad(guild store.GuildID, ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.rolesGate.finish(guild, ok)
}

func (a *App) beginChannelLoad(guild store.GuildID) bool {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	return a.channelsGate.begin(guild)
}

func (a *App) finishChannelLoad(guild store.GuildID, ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.channelsGate.finish(guild, ok)
}

// Connect opens the gateway and blocks until ctx is canceled.
func (a *App) Connect(ctx context.Context) error {
	return a.handle.Connect(ctx)
}
