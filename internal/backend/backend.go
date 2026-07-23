// Package backend defines the protocol-neutral orchestrator surface the UI,
// account manager, and plugin host depend on. Discord (internal/app) and Matrix
// (internal/matrixapp) each implement Backend, so the rest of the client speaks
// only store.* types and this interface — never a concrete protocol library.
//
// Every callback registered through the On* setters and every Post closure runs
// on the single UI goroutine; implementations must preserve that discipline
// (see the FIFO ingestion note on internal/app's gateway handlers).
package backend

import (
	"context"
	"io"

	"awesomeProject/internal/store"
)

// DirectMessagesGuildID is the synthetic guild that owns private channels/DMs in
// the UI. It avoids overloading guild ID 0, which orchestrators use as "not
// selected".
const DirectMessagesGuildID store.GuildID = ^store.GuildID(0)

// UploadFile is a protocol-neutral outgoing attachment. Each backend converts it
// to its own transport type (Discord multipart, Matrix upload) inside SendFiles.
type UploadFile struct {
	Name   string
	Reader io.Reader
}

// EventSink receives client events for out-of-tree consumers (the Lua plugin
// system). Emit must not block — implementations enqueue and return. Payload
// snowflake/ID fields are uint64.
type EventSink interface {
	Emit(name string, data map[string]any)
}

// StateSnapshot is an immutable, concurrently readable view of the small piece
// of UI-owned state exposed to integrations. Read it via Backend.Snapshot from a
// background goroutine instead of ActiveGuild/ActiveChannel/SelfID.
type StateSnapshot struct {
	ActiveGuild   store.GuildID
	ActiveChannel store.ChannelID
	SelfID        store.UserID
}

// UnreadStatus is the server-authoritative attention state for a guild or
// channel. Mentions take precedence over ordinary unread messages.
type UnreadStatus uint8

const (
	Read UnreadStatus = iota
	Unread
	Mentioned
)

// Backend is the orchestrator surface shared by every chat protocol. Method
// semantics match internal/app.App exactly. Methods that a given protocol does
// not support (e.g. Discord role/channel management on Matrix) are implemented
// as no-ops rather than omitted, so the UI needs no per-protocol branching.
//
// Protocol-specific surfaces that reference concrete protocol types (Discord
// slash commands, message components, GIF search) are deliberately NOT here;
// callers reach them by type-asserting the concrete backend.
type Backend interface {
	// Runtime / lifecycle.
	Store() *store.Store
	Connect(ctx context.Context) error // blocks until ctx is canceled; owns the reconnect loop
	RegisterHandlers()
	Post(func())
	TryPost(func()) bool
	WriteRaw([]byte)
	Invalidate()
	ForceRepaint()

	// Identity & selection.
	ActiveGuild() store.GuildID
	ActiveChannel() store.ChannelID
	SelfID() store.UserID
	Self() (store.Member, bool)
	Snapshot() StateSnapshot
	SetActive(guild store.GuildID, channel store.ChannelID)

	// Messaging.
	Send(content string)
	SendFiles(content string, files []UploadFile, optimistic []store.Attachment, cleanup func())
	SendToChannel(channel store.ChannelID, content string)
	Reply(content string, message store.Message, mention bool)
	SendSticker(id uint64)
	EditMessage(channel store.ChannelID, id store.MessageID, content string)
	DeleteMessage(channel store.ChannelID, id store.MessageID)
	AddReaction(channel store.ChannelID, id store.MessageID, emoji string)
	SetPinned(channel store.ChannelID, id store.MessageID, pinned bool)

	// Loading.
	LoadGuilds(limit uint)
	LoadChannels(guild store.GuildID)
	LoadHistory(channel store.ChannelID, limit uint)
	LoadOlderHistory(channel store.ChannelID)
	LoadRoles(guild store.GuildID)
	LoadForumMetadata(channel store.ChannelID)
	EnsureMemberDetail(guild store.GuildID, user store.UserID, done func())

	// Threads.
	LoadActiveThreads(guild store.GuildID)
	LoadArchivedThreads(channel store.ChannelID)
	CreateThreadFromMessage(channel store.ChannelID, message store.MessageID, name string)
	JoinThread(thread store.ChannelID)
	LeaveThread(thread store.ChannelID)
	SetThreadArchived(thread store.ChannelID, archived bool)
	Publish(channel store.ChannelID, message store.MessageID)
	CreateForumPost(forum store.ChannelID, title, body string, tagIDs []uint64)

	// Roles & channel management (Discord-only; no-op on protocols without them).
	CreateRole(guild store.GuildID, name string)
	RenameRole(guild store.GuildID, role store.RoleID, name string)
	SetRoleColor(guild store.GuildID, role store.RoleID, color uint32)
	SetRoleHoist(guild store.GuildID, role store.RoleID, value bool)
	SetRoleMentionable(guild store.GuildID, role store.RoleID, value bool)
	DeleteRole(guild store.GuildID, role store.RoleID)
	MoveRole(guild store.GuildID, role store.RoleID, position int)
	CreateTextChannel(guild store.GuildID, name string)
	RenameChannel(id store.ChannelID, name string)
	DeleteChannel(id store.ChannelID)
	MoveChannel(guild store.GuildID, id store.ChannelID, position int)

	// Read state.
	Unread() (unread bool, mentions int)
	ChannelUnread(channel store.ChannelID) UnreadStatus
	GuildUnread(guild store.GuildID) UnreadStatus
	MarkRead(channel store.ChannelID, message store.MessageID)
	MarkChannelRead(channel store.ChannelID)

	// Callbacks (each runs on the UI goroutine).
	OnReady(func())
	OnChange(func())
	OnGuildChange(func())
	OnReadStateChange(func())
	OnIncomingMessage(func(store.Message))
	OnError(func(error))
	SetEventSink(EventSink)
}
