package matrixapp

import (
	"context"
	"errors"
	"mime"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/attachment"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"awesomeProject/internal/backend"
	"awesomeProject/internal/store"
)

// errEncryptionUnavailable is surfaced when the user tries to post to an
// encrypted room while E2EE is not working. Sending would go out as plaintext
// (mautrix only encrypts when its crypto state store knows the room is
// encrypted, which never gets populated after a crypto init failure), so we
// refuse rather than silently leak the message into an "encrypted" room.
var errEncryptionUnavailable = errors.New("can't send: this room is end-to-end encrypted but encryption isn't set up on this device — see the sign-in warning. Refusing to send unencrypted")

// blockedByEncryption reports whether a post to channel must be refused because
// the room is encrypted but E2EE is not functional. It also surfaces the reason.
func (a *App) blockedByEncryption(channel store.ChannelID) bool {
	if a.cryptoReady.Load() || !a.roomEncrypted(channel) {
		return false
	}
	a.reportError(errEncryptionUnavailable)
	return true
}

// roomEncrypted reports whether the room behind channel has advertised
// m.room.encryption.
func (a *App) roomEncrypted(channel store.ChannelID) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	room, ok := a.roomByChannel[channel]
	if !ok {
		return false
	}
	info := a.rooms[room]
	return info != nil && info.encrypted
}

// resolveRoom maps a store channel back to its Matrix room ID.
func (a *App) resolveRoom(channel store.ChannelID) (id.RoomID, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	room, ok := a.roomByChannel[channel]
	return room, ok
}

// resolveEvent maps a store message ID back to its Matrix event ID.
func (a *App) resolveEvent(id store.MessageID) (string, bool) {
	return a.ids.str(uint64(id))
}

// --- sending ----------------------------------------------------------------

func (a *App) Send(content string) {
	a.SendToChannel(a.activeChannel, content)
}

func (a *App) SendToChannel(channel store.ChannelID, content string) {
	if strings.TrimSpace(content) == "" || channel == 0 {
		return
	}
	room, ok := a.resolveRoom(channel)
	if !ok {
		return
	}
	if a.blockedByEncryption(channel) {
		return
	}
	txn := a.client.M.TxnID()
	a.store.AppendMessage(store.Message{
		ChannelID: channel,
		AuthorID:  a.self.ID,
		Author:    a.selfName(),
		Content:   content,
		Nonce:     txn,
		Pending:   true,
	})
	msg := &event.MessageEventContent{MsgType: event.MsgText, Body: content}
	go a.deliver(room, channel, txn, msg)
}

func (a *App) SendFiles(content string, files []backend.UploadFile, optimistic []store.Attachment, cleanup func()) {
	channel := a.activeChannel
	if channel == 0 || (strings.TrimSpace(content) == "" && len(files) == 0) {
		if cleanup != nil {
			cleanup()
		}
		return
	}
	room, ok := a.resolveRoom(channel)
	if !ok {
		if cleanup != nil {
			cleanup()
		}
		return
	}
	if a.blockedByEncryption(channel) {
		if cleanup != nil {
			cleanup()
		}
		return
	}
	if strings.TrimSpace(content) != "" {
		a.SendToChannel(channel, content)
	}
	fileCopy := append([]backend.UploadFile(nil), files...)
	go func() {
		if cleanup != nil {
			defer cleanup()
		}
		for _, f := range fileCopy {
			a.uploadAndSend(room, channel, f)
		}
	}()
}

// deliver sends a message event and reconciles the optimistic echo on failure.
// On success the reconciliation happens when the event echoes back through sync
// (matched by transaction ID).
func (a *App) deliver(room id.RoomID, channel store.ChannelID, txn string, content *event.MessageEventContent) {
	_, err := a.client.M.SendMessageEvent(context.Background(), room, event.EventMessage, content, mautrix.ReqSendEvent{TransactionID: txn})
	if err != nil {
		a.ui.Post(func() {
			a.store.MarkFailed(channel, txn)
			a.fireChange()
		})
		a.reportError(err)
	}
}

func (a *App) uploadAndSend(room id.RoomID, channel store.ChannelID, f backend.UploadFile) {
	data, err := readAll(f.Reader)
	if err != nil {
		a.reportError(err)
		return
	}
	ctype := mime.TypeByExtension(filepath.Ext(f.Name))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	content := &event.MessageEventContent{
		MsgType: mediaMsgType(ctype),
		Body:    f.Name,
		Info:    &event.FileInfo{MimeType: ctype, Size: len(data)},
	}

	// In an encrypted room the media itself must be encrypted before upload —
	// SendMessageEvent only encrypts the event JSON, not the bytes the
	// homeserver stores. Upload ciphertext and carry the keys in content.File.
	ctx := context.Background()
	encrypted := false
	if a.client.M.StateStore != nil {
		if enc, encErr := a.client.M.StateStore.IsEncrypted(ctx, room); encErr == nil {
			encrypted = enc
		}
	}
	if encrypted {
		ef := attachment.NewEncryptedFile()
		ciphertext := ef.Encrypt(data)
		resp, upErr := a.client.M.UploadBytes(ctx, ciphertext, "application/octet-stream")
		if upErr != nil {
			a.reportError(upErr)
			return
		}
		content.File = &event.EncryptedFileInfo{EncryptedFile: *ef, URL: resp.ContentURI.CUString()}
	} else {
		resp, upErr := a.client.M.UploadBytesWithName(ctx, data, ctype, f.Name)
		if upErr != nil {
			a.reportError(upErr)
			return
		}
		content.URL = resp.ContentURI.CUString()
	}
	a.deliver(room, channel, a.client.M.TxnID(), content)
}

func mediaMsgType(ctype string) event.MessageType {
	switch {
	case strings.HasPrefix(ctype, "image/"):
		return event.MsgImage
	case strings.HasPrefix(ctype, "video/"):
		return event.MsgVideo
	case strings.HasPrefix(ctype, "audio/"):
		return event.MsgAudio
	default:
		return event.MsgFile
	}
}

func (a *App) Reply(content string, message store.Message, mention bool) {
	if strings.TrimSpace(content) == "" {
		return
	}
	channel := message.ChannelID
	room, ok := a.resolveRoom(channel)
	if !ok {
		return
	}
	targetID, ok := a.resolveEvent(message.ID)
	if !ok {
		a.SendToChannel(channel, content)
		return
	}
	txn := a.client.M.TxnID()
	a.store.AppendMessage(store.Message{
		ChannelID: channel,
		AuthorID:  a.self.ID,
		Author:    a.selfName(),
		Content:   content,
		Nonce:     txn,
		Pending:   true,
		Reply:     &store.MessageReply{MessageID: message.ID, ChannelID: channel, Author: message.Author, Content: message.Content},
	})
	msg := &event.MessageEventContent{MsgType: event.MsgText, Body: content}
	msg.SetReply(&event.Event{ID: id.EventID(targetID), RoomID: room})
	go a.deliver(room, channel, txn, msg)
}

func (a *App) EditMessage(channel store.ChannelID, id store.MessageID, content string) {
	room, ok := a.resolveRoom(channel)
	if !ok {
		return
	}
	targetID, ok := a.resolveEvent(id)
	if !ok {
		return
	}
	msg := &event.MessageEventContent{MsgType: event.MsgText, Body: content}
	msg.SetEdit(eventID(targetID))
	go a.deliver(room, channel, a.client.M.TxnID(), msg)
}

func (a *App) DeleteMessage(channel store.ChannelID, id store.MessageID) {
	room, ok := a.resolveRoom(channel)
	if !ok {
		return
	}
	targetID, ok := a.resolveEvent(id)
	if !ok {
		return
	}
	go func() {
		if _, err := a.client.M.RedactEvent(context.Background(), room, eventID(targetID)); err != nil {
			a.reportError(err)
		}
	}()
}

func (a *App) AddReaction(channel store.ChannelID, id store.MessageID, emoji string) {
	room, ok := a.resolveRoom(channel)
	if !ok {
		return
	}
	targetID, ok := a.resolveEvent(id)
	if !ok {
		return
	}
	go func() {
		if _, err := a.client.M.SendReaction(context.Background(), room, eventID(targetID), emoji); err != nil {
			a.reportError(err)
		}
	}()
}

// SetPinned toggles a message in the room's m.room.pinned_events state.
func (a *App) SetPinned(channel store.ChannelID, id store.MessageID, pinned bool) {
	room, ok := a.resolveRoom(channel)
	if !ok {
		return
	}
	targetID, ok := a.resolveEvent(id)
	if !ok {
		return
	}
	go func() {
		var current event.PinnedEventsEventContent
		_ = a.client.M.StateEvent(context.Background(), room, event.StatePinnedEvents, "", &current)
		next := current.Pinned[:0:0]
		found := false
		for _, e := range current.Pinned {
			if e == eventID(targetID) {
				found = true
				if pinned {
					next = append(next, e)
				}
				continue
			}
			next = append(next, e)
		}
		if pinned && !found {
			next = append(next, eventID(targetID))
		}
		if _, err := a.client.M.SendStateEvent(context.Background(), room, event.StatePinnedEvents, "", &event.PinnedEventsEventContent{Pinned: next}); err != nil {
			a.reportError(err)
		}
	}()
}

// --- read state -------------------------------------------------------------

func (a *App) MarkRead(channel store.ChannelID, message store.MessageID) {
	room, ok := a.resolveRoom(channel)
	if !ok {
		return
	}
	targetID, ok := a.resolveEvent(message)
	if !ok {
		return
	}
	go func() {
		err := a.client.M.SetReadMarkers(context.Background(), room, &mautrix.ReqSetReadMarkers{
			Read:      eventID(targetID),
			FullyRead: eventID(targetID),
		})
		if err != nil {
			a.reportError(err)
		}
	}()
}

func (a *App) MarkChannelRead(channel store.ChannelID) {
	if last, ok := a.store.LastMsg(channel); ok && last.ID != 0 {
		a.MarkRead(channel, last.ID)
	}
	a.store.ClearUnread(channel)
	a.mu.Lock()
	delete(a.channelUnread, channel)
	delete(a.channelHighlight, channel)
	a.recomputeAggregatesLocked()
	a.mu.Unlock()
}

// --- loading ----------------------------------------------------------------

// LoadGuilds is a no-op: the sync loop delivers the room directory. It exists so
// the account manager's pre-connect pull is harmless on Matrix.
func (a *App) LoadGuilds(limit uint) {}

// LoadChannels is a no-op: rooms arrive through sync, not per-guild REST.
func (a *App) LoadChannels(guild store.GuildID) {}

// LoadHistory backfills one page of older messages above the synced timeline.
func (a *App) LoadHistory(channel store.ChannelID, limit uint) {
	a.backfill(channel, int(limit))
}

// LoadOlderHistory continues backfilling older messages.
func (a *App) LoadOlderHistory(channel store.ChannelID) {
	a.backfill(channel, 50)
}

// backfillGate serializes backfill per channel so overlapping LoadHistory /
// LoadOlderHistory calls (or rapid scrolling) cannot fetch the same page from
// the same prev_batch token twice and prepend duplicates.
var backfillGate sync.Map // store.ChannelID -> *int32 (0 idle, 1 in-flight)

func (a *App) backfill(channel store.ChannelID, limit int) {
	room, ok := a.resolveRoom(channel)
	if !ok {
		return
	}
	gateAny, _ := backfillGate.LoadOrStore(channel, new(int32))
	gate := gateAny.(*int32)
	if !atomic.CompareAndSwapInt32(gate, 0, 1) {
		return // a backfill for this channel is already running
	}
	a.mu.Lock()
	info := a.rooms[room]
	from := ""
	if info != nil {
		from = info.prevBatch
	}
	a.mu.Unlock()
	if from == "" {
		atomic.StoreInt32(gate, 0)
		return
	}
	if limit <= 0 {
		limit = 50
	}
	go func() {
		defer atomic.StoreInt32(gate, 0)
		resp, err := a.client.M.Messages(context.Background(), room, from, "", mautrix.DirectionBackward, nil, limit)
		if err != nil {
			a.reportError(err)
			return
		}
		// Chunk is newest-first; reverse to chronological for prepend.
		msgs := make([]store.Message, 0, len(resp.Chunk))
		for i := len(resp.Chunk) - 1; i >= 0; i-- {
			evt := resp.Chunk[i]
			if evt.Type != event.EventMessage {
				continue
			}
			evt.Content.ParseRaw(evt.Type)
			content := evt.Content.AsMessage()
			if content == nil || (content.RelatesTo != nil && content.RelatesTo.Type == event.RelReplace) {
				continue
			}
			msgs = append(msgs, a.convertMessage(evt, content, channel))
		}
		a.mu.Lock()
		if info != nil {
			info.prevBatch = resp.End
		}
		a.mu.Unlock()
		a.ui.Post(func() {
			a.store.PrependMessages(channel, msgs)
			a.fireChange()
		})
	}()
}

func (a *App) selfName() string {
	if a.self.Name != "" {
		return a.self.Name
	}
	return localpart(a.selfID)
}

func eventID(s string) id.EventID { return id.EventID(s) }
