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
	"strconv"
	"strings"
	"sync"

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
)

// poster is the slice of tui.App the orchestrator depends on. It exists so the
// orchestration logic can be tested without a real terminal runtime.
type poster interface {
	Post(func())
}

// sender is the slice of the arikawa client used to send messages.
type sender interface {
	SendMessageComplex(discord.ChannelID, api.SendMessageData) (*discord.Message, error)
	EditText(discord.ChannelID, discord.MessageID, string) (*discord.Message, error)
	DeleteMessage(discord.ChannelID, discord.MessageID, api.AuditLogReason) error
	PinMessage(discord.ChannelID, discord.MessageID, api.AuditLogReason) error
	UnpinMessage(discord.ChannelID, discord.MessageID, api.AuditLogReason) error
	CrosspostMessage(discord.ChannelID, discord.MessageID) (*discord.Message, error)
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
}

type roleLoader interface {
	Roles(discord.GuildID) ([]discord.Role, error)
}

type directoryLoader interface {
	Guilds(uint) ([]discord.Guild, error)
	PrivateChannels() ([]discord.Channel, error)
}

type channelLoader interface {
	Channels(discord.GuildID) ([]discord.Channel, error)
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

// restInteractionPoster posts interactions through the session's REST client.
type restInteractionPoster struct {
	sess *session.Session
}

func (r restInteractionPoster) postComponentInteraction(p componentInteraction) error {
	url := strings.TrimSuffix(api.EndpointInteractions, "/")
	return r.sess.FastRequest("POST", url, httputil.WithJSONBody(p))
}

// App wires the session, store, and UI together and tracks navigation state.
type App struct {
	store    *store.Store
	ui       poster
	send     sender
	history  historyLoader
	roles    roleLoader
	dirs     directoryLoader
	chans    channelLoader
	threads  threadClient
	forum    forumPoster
	interact componentInteractionPoster
	handle   *session.Session

	resourceMu      sync.Mutex
	historyLoaded   map[store.ChannelID]struct{}
	historyPending  map[store.ChannelID]struct{}
	rolesLoaded     map[store.GuildID]struct{}
	rolesPending    map[store.GuildID]struct{}
	guildsLoaded    bool
	guildsPending   bool
	channelsLoaded  map[store.GuildID]struct{}
	channelsPending map[store.GuildID]struct{}
	threadsLoaded   map[store.GuildID]struct{}
	threadsPending  map[store.GuildID]struct{}
	archivedLoaded  map[store.ChannelID]struct{}
	archivedPending map[store.ChannelID]struct{}

	onReady  func()
	onChange func()
	onError  func(error)

	activeGuild   store.GuildID
	activeChannel store.ChannelID
	selfID        store.UserID
	// sessionID is the gateway session identifier from READY; Discord requires
	// it on user-originated interaction payloads.
	sessionID string
}

// New returns an orchestrator over the given session, store, and UI runtime.
func New(sess *session.Session, st *store.Store, ui *tui.App) *App {
	return &App{
		store:    st,
		ui:       ui,
		send:     sess,
		history:  sess,
		roles:    sess,
		dirs:     sess,
		chans:    sess,
		threads:  sess,
		forum:    restForumPoster{sess: sess},
		interact: restInteractionPoster{sess: sess},
		handle:   sess,
	}
}

func (a *App) ensureResourceMaps() {
	if a.historyLoaded == nil {
		a.historyLoaded = map[store.ChannelID]struct{}{}
	}
	if a.historyPending == nil {
		a.historyPending = map[store.ChannelID]struct{}{}
	}
	if a.rolesLoaded == nil {
		a.rolesLoaded = map[store.GuildID]struct{}{}
	}
	if a.rolesPending == nil {
		a.rolesPending = map[store.GuildID]struct{}{}
	}
	if a.channelsLoaded == nil {
		a.channelsLoaded = map[store.GuildID]struct{}{}
	}
	if a.channelsPending == nil {
		a.channelsPending = map[store.GuildID]struct{}{}
	}
	if a.threadsLoaded == nil {
		a.threadsLoaded = map[store.GuildID]struct{}{}
	}
	if a.threadsPending == nil {
		a.threadsPending = map[store.GuildID]struct{}{}
	}
	if a.archivedLoaded == nil {
		a.archivedLoaded = map[store.ChannelID]struct{}{}
	}
	if a.archivedPending == nil {
		a.archivedPending = map[store.ChannelID]struct{}{}
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
}

// OnReady registers a callback run (on the UI goroutine) after the READY event
// has populated the store, so the UI can select an initial channel.
func (a *App) OnReady(fn func()) { a.onReady = fn }

// OnChange registers a callback run (on the UI goroutine) after an incoming
// message updates the store, so the UI can refresh unread badges.
func (a *App) OnChange(fn func()) { a.onChange = fn }

// OnError registers a callback run (on the UI goroutine) when background work
// fails but the client can keep running.
func (a *App) OnError(fn func(error)) { a.onError = fn }

// RegisterHandlers subscribes to the gateway events the client consumes. Each
// handler marshals its store mutation onto the UI goroutine via Post.
func (a *App) RegisterHandlers() {
	a.handle.AddHandler(func(e *gateway.ReadyEvent) {
		a.handleReady(e)
	})

	a.handle.AddHandler(func(e *gateway.GuildCreateEvent) {
		a.handleGuildCreate(e)
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
			a.store.RemoveRole(guildID, roleID)
			if a.onChange != nil {
				a.onChange()
			}
		})
	})

	a.handle.AddHandler(func(e *gateway.ThreadCreateEvent) {
		a.handleThreadUpsert(e.Channel)
	})

	a.handle.AddHandler(func(e *gateway.ThreadUpdateEvent) {
		a.handleThreadUpsert(e.Channel)
	})

	a.handle.AddHandler(func(e *gateway.ThreadDeleteEvent) {
		id := store.ChannelID(e.ID)
		a.ui.Post(func() {
			a.store.RemoveThread(id)
			if a.onChange != nil {
				a.onChange()
			}
		})
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
	}
	a.ui.Post(func() {
		if member != nil {
			a.store.UpsertMember(store.GuildID(e.GuildID), convertMember(*member))
		}
		// Reconcile an optimistic local echo when possible; otherwise append.
		if !a.store.ReplaceMessage(msg.Nonce, msg) {
			a.store.AppendMessage(msg)
			if msg.ChannelID != a.activeChannel {
				a.store.IncrementUnread(msg.ChannelID)
			}
		}
		if a.onChange != nil {
			a.onChange()
		}
	})
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
			m.Attachments = patch.Attachments
			m.Embeds = patch.Embeds
			m.Stickers = patch.Stickers
			m.Components = patch.Components
			m.Pinned = patch.Pinned
		})
		if a.onChange != nil {
			a.onChange()
		}
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
	a.ui.Post(func() {
		a.store.AddReaction(channel, id, react)
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) handleReactionRemove(e *gateway.MessageReactionRemoveEvent) {
	channel := store.ChannelID(e.ChannelID)
	id := store.MessageID(e.MessageID)
	name := e.Emoji.Name
	emojiID := uint64(e.Emoji.ID)
	me := store.UserID(e.UserID) == a.selfID && a.selfID != 0
	a.ui.Post(func() {
		a.store.RemoveReaction(channel, id, name, emojiID, me)
		if a.onChange != nil {
			a.onChange()
		}
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
			a.store.UpsertMember(guildID, convertMember(member))
		}
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) handleMemberUpsert(guild discord.GuildID, member discord.Member) {
	guildID := store.GuildID(guild)
	a.ui.Post(func() {
		a.store.UpsertMember(guildID, convertMember(member))
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
	if content == "" || a.activeChannel == 0 {
		return
	}
	channel := a.activeChannel
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

// Reply sends content as a Discord inline reply to message.
func (a *App) Reply(content string, message store.Message, mention bool) {
	if content == "" || message.ChannelID == 0 || message.ID == 0 {
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
	go func() {
		_, err := a.send.EditText(discord.ChannelID(channel), discord.MessageID(id), content)
		if err != nil {
			a.reportError(err)
		}
	}()
}

// DeleteMessage deletes a message. Local removal waits for MESSAGE_DELETE.
func (a *App) DeleteMessage(channel store.ChannelID, id store.MessageID) {
	if channel == 0 || id == 0 {
		return
	}
	go func() {
		if err := a.send.DeleteMessage(discord.ChannelID(channel), discord.MessageID(id), ""); err != nil {
			a.reportError(err)
		}
	}()
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
		a.store.SetMessages(channel, converted)
		a.finishHistoryLoad(channel, true)
		if a.onChange != nil {
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
	a.rolesLoaded[guild] = struct{}{}
	delete(a.rolesPending, guild)
}

func (a *App) markChannelsLoaded(guild store.GuildID) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	a.channelsLoaded[guild] = struct{}{}
	delete(a.channelsPending, guild)
}

func (a *App) beginGuildLoad() bool {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	if a.guildsLoaded || a.guildsPending {
		return false
	}
	a.guildsPending = true
	return true
}

func (a *App) finishGuildLoad(ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.guildsPending = false
	if ok {
		a.guildsLoaded = true
	}
}

func (a *App) beginHistoryLoad(channel store.ChannelID) bool {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	if _, ok := a.historyLoaded[channel]; ok {
		return false
	}
	if _, ok := a.historyPending[channel]; ok {
		return false
	}
	a.historyPending[channel] = struct{}{}
	return true
}

func (a *App) finishHistoryLoad(channel store.ChannelID, ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	delete(a.historyPending, channel)
	if ok {
		a.historyLoaded[channel] = struct{}{}
	}
}

func (a *App) beginRoleLoad(guild store.GuildID) bool {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	if _, ok := a.rolesLoaded[guild]; ok {
		return false
	}
	if _, ok := a.rolesPending[guild]; ok {
		return false
	}
	a.rolesPending[guild] = struct{}{}
	return true
}

func (a *App) finishRoleLoad(guild store.GuildID, ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	delete(a.rolesPending, guild)
	if ok {
		a.rolesLoaded[guild] = struct{}{}
	}
}

func (a *App) beginChannelLoad(guild store.GuildID) bool {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	if _, ok := a.channelsLoaded[guild]; ok {
		return false
	}
	if _, ok := a.channelsPending[guild]; ok {
		return false
	}
	a.channelsPending[guild] = struct{}{}
	return true
}

func (a *App) finishChannelLoad(guild store.GuildID, ok bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	a.ensureResourceMaps()
	delete(a.channelsPending, guild)
	if ok {
		a.channelsLoaded[guild] = struct{}{}
	}
}

// Connect opens the gateway and blocks until ctx is canceled.
func (a *App) Connect(ctx context.Context) error {
	return a.handle.Connect(ctx)
}
