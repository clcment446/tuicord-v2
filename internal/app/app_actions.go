// Package app orchestrates the Discord session, the normalized store, and the TUI runtime.
package app

import (
	"awesomeProject/internal/store"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
	"github.com/diamondburned/arikawa/v3/utils/sendpart"
	"strconv"
	"strings"
	"time"
)

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
	if !hasSubmittedComponent(msg.ComponentTree, componentType, sub.CustomID) {
		return
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

// hasSubmittedComponent prevents callers (including plugins) from inventing a
// component interaction or changing a select into a button. Component trees
// are recursive because Components V2 controls may live inside containers or
// section accessories.
func hasSubmittedComponent(nodes []store.ComponentNode, componentType int, customID string) bool {
	for _, node := range nodes {
		nodeType := node.RawType
		if nodeType == 0 {
			switch node.Kind {
			case store.ComponentButton:
				nodeType = 2
			case store.ComponentSelect:
				nodeType = 3
			}
		}
		if nodeType == componentType && node.CustomID == customID && !node.Disabled {
			return true
		}
		if hasSubmittedComponent(node.Children, componentType, customID) {
			return true
		}
		if node.Accessory != nil && hasSubmittedComponent([]store.ComponentNode{*node.Accessory}, componentType, customID) {
			return true
		}
	}
	return false
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
	msg, err := a.send.SendMessageComplex(discord.ChannelID(channel), data)
	if err != nil {
		a.ui.Post(func() {
			a.store.MarkFailed(channel, nonce)
			if a.onError != nil {
				a.onError(err)
			}
		})
		return
	}
	if msg == nil {
		return
	}
	// Confirm the optimistic echo from the REST response rather than depending on
	// the gateway MESSAGE_CREATE, which can be dropped or lost across a reconnect —
	// otherwise the message stays "pending" forever. A later duplicate gateway
	// echo re-matches the same nonce (or is caught by the HasMessage guard), so it
	// never doubles the message.
	confirmed := convertMessage(*msg)
	if confirmed.ID == 0 {
		// No usable id in the response; fall back to the gateway echo.
		return
	}
	a.ui.Post(func() {
		if !a.store.ReplaceMessage(nonce, confirmed) && !a.store.HasMessage(channel, confirmed.ID) {
			a.store.AppendMessage(confirmed)
		}
		if a.onChange != nil {
			a.onChange()
		}
	})
}

// EditMessage patches a message's content. The edit is applied from the REST
// response so the visible message updates even if the MESSAGE_UPDATE echo is
// missed; failures are reported via OnError.
func (a *App) EditMessage(channel store.ChannelID, id store.MessageID, content string) {
	if channel == 0 || id == 0 {
		return
	}
	go func() {
		resp, err := a.send.EditText(discord.ChannelID(channel), discord.MessageID(id), content)
		if err != nil {
			a.reportAsyncError(err)
			return
		}
		if resp == nil {
			return
		}
		// Confirm from the authoritative REST response, mirroring deliver: the body
		// carries the server's committed content and edit timestamp, so two rapid
		// edits whose REST calls complete out of order still converge on the server's
		// last write instead of whichever goroutine reaches the UI last.
		confirmed := convertMessage(*resp)
		edited := resp.EditedTimestamp.Time()
		a.ui.Post(func() {
			a.applyEditConfirmation(channel, id, confirmed.Content, edited)
		})
	}()
}

// applyEditConfirmation writes an authoritative edit response into the store on
// the UI goroutine. The HasMessage guard prevents a delete that already landed
// from being undone (delete-then-edit must not resurrect the message). The
// edit-timestamp check keeps the store monotonic: when two edits complete out of
// order, a response older than the last one applied is dropped, so both paths
// settle on the server's final content. editApplied is UI-goroutine owned, so it
// needs no lock.
func (a *App) applyEditConfirmation(channel store.ChannelID, id store.MessageID, content string, edited time.Time) {
	if a == nil || a.store == nil || !a.store.HasMessage(channel, id) {
		return
	}
	if a.editApplied == nil {
		a.editApplied = make(map[store.ChannelID]map[store.MessageID]time.Time)
	}
	applied := a.editApplied[channel]
	if applied == nil {
		applied = make(map[store.MessageID]time.Time)
		a.editApplied[channel] = applied
	}
	// A response that omitted the edit timestamp (zero) still applies the first
	// time; only a stamp strictly older than one already applied is rejected.
	if prev, ok := applied[id]; ok && !edited.IsZero() && edited.Before(prev) {
		return
	}
	applied[id] = edited
	a.store.UpdateMessage(channel, id, func(m *store.Message) { m.Content = content })
	if a.onChange != nil {
		a.onChange()
	}
}

// DeleteMessage deletes a message. The local removal is applied on REST success
// so the message disappears even if the MESSAGE_DELETE echo is missed.
func (a *App) DeleteMessage(channel store.ChannelID, id store.MessageID) {
	if channel == 0 || id == 0 {
		return
	}
	go func() {
		if err := a.send.DeleteMessage(discord.ChannelID(channel), discord.MessageID(id), ""); err != nil {
			a.reportAsyncError(err)
			return
		}
		a.ui.Post(func() {
			a.store.RemoveMessage(channel, id)
			if a.onChange != nil {
				a.onChange()
			}
		})
	}()
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
	a.runMutation(func() error {
		var err error
		if pinned {
			err = a.send.PinMessage(discord.ChannelID(channel), discord.MessageID(id), "")
		} else {
			err = a.send.UnpinMessage(discord.ChannelID(channel), discord.MessageID(id), "")
		}
		return err
	}, func() {
		a.store.SetMessagePinned(channel, id, pinned)
		if a.onChange != nil {
			a.onChange()
		}
	})
}

func (a *App) reportError(err error) {
	a.reportAsyncError(err)
}
