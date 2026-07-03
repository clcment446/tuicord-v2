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

	"awesomeProject/internal/store"
	"awesomeProject/internal/tui/tui"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/session"
)

// poster is the slice of tui.App the orchestrator depends on. It exists so the
// orchestration logic can be tested without a real terminal runtime.
type poster interface {
	Post(func())
}

// sender is the slice of the arikawa client used to send messages.
type sender interface {
	SendMessageComplex(discord.ChannelID, api.SendMessageData) (*discord.Message, error)
}

// App wires the session, store, and UI together and tracks navigation state.
type App struct {
	store  *store.Store
	ui     poster
	send   sender
	handle *session.Session

	onReady  func()
	onChange func()

	activeGuild   store.GuildID
	activeChannel store.ChannelID
}

// New returns an orchestrator over the given session, store, and UI runtime.
func New(sess *session.Session, st *store.Store, ui *tui.App) *App {
	return &App{
		store:  st,
		ui:     ui,
		send:   sess,
		handle: sess,
	}
}

// Store returns the underlying state store (read on the UI goroutine).
func (a *App) Store() *store.Store { return a.store }

// ActiveGuild returns the currently selected guild.
func (a *App) ActiveGuild() store.GuildID { return a.activeGuild }

// ActiveChannel returns the currently selected channel.
func (a *App) ActiveChannel() store.ChannelID { return a.activeChannel }

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

// RegisterHandlers subscribes to the gateway events the client consumes. Each
// handler marshals its store mutation onto the UI goroutine via Post.
func (a *App) RegisterHandlers() {
	a.handle.AddHandler(func(e *gateway.ReadyEvent) {
		guilds := e.Guilds
		a.ui.Post(func() {
			for i := range guilds {
				ingestGuild(a.store, &guilds[i])
			}
			if a.onReady != nil {
				a.onReady()
			}
		})
	})

	a.handle.AddHandler(func(e *gateway.GuildCreateEvent) {
		guild := *e
		a.ui.Post(func() {
			ingestGuild(a.store, &guild)
		})
	})

	a.handle.AddHandler(func(e *gateway.MessageCreateEvent) {
		msg := convertMessage(e.Message)
		a.ui.Post(func() {
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

	go a.deliver(channel, content, nonce)
}

func (a *App) deliver(channel store.ChannelID, content, nonce string) {
	_, err := a.send.SendMessageComplex(discord.ChannelID(channel), api.SendMessageData{
		Content: content,
		Nonce:   nonce,
	})
	if err != nil {
		a.ui.Post(func() {
			a.store.MarkFailed(channel, nonce)
		})
	}
}

// Connect opens the gateway and blocks until ctx is canceled.
func (a *App) Connect(ctx context.Context) error {
	return a.handle.Connect(ctx)
}
