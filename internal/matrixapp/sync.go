package matrixapp

import (
	"context"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"awesomeProject/internal/backend"
	"awesomeProject/internal/matrix"
	"awesomeProject/internal/store"
)

// RegisterHandlers subscribes to the gateway events the client consumes. Each
// handler marshals its store mutation onto the UI goroutine via Post. The crypto
// helper (installed in matrix.New) decrypts m.room.encrypted events and
// re-dispatches them as plaintext, so these handlers see decrypted content.
func (a *App) RegisterHandlers() {
	s := a.client.Syncer()
	s.ParseEventContent = true

	s.OnEventType(event.StateCreate, a.onStateCreate)
	s.OnEventType(event.StateRoomName, a.onStateName)
	s.OnEventType(event.StateTopic, a.onStateTopic)
	s.OnEventType(event.StateRoomAvatar, a.onStateAvatar)
	s.OnEventType(event.StateMember, a.onStateMember)
	s.OnEventType(event.StateSpaceChild, a.onSpaceChild)
	s.OnEventType(event.AccountDataDirectChats, a.onDirectChats)

	s.OnEventType(event.EventMessage, a.onMessage)
	s.OnEventType(event.EventReaction, a.onReaction)
	s.OnEventType(event.EventRedaction, a.onRedaction)

	// Surface undecryptable messages as a placeholder instead of dropping them
	// silently (missing keys, unverified session). If keys arrive later, mautrix
	// re-decrypts and re-dispatches, replacing the placeholder via its event ID.
	if a.client.Crypto != nil {
		a.client.Crypto.DecryptErrorCallback = a.onDecryptError
	}

	// Per-room unread counts and pagination tokens live only on the sync
	// response, not on individual events. Returning true keeps the crypto helper
	// and per-event dispatch running.
	s.OnSync(a.onSync)
}

// Connect opens the sync loop and blocks until ctx is canceled. It owns the
// reconnect loop: mautrix's SyncWithContext returns on error, so we back off and
// retry, mirroring the Discord orchestrator's Connect contract.
func (a *App) Connect(ctx context.Context) error {
	// Load E2EE state (device keys, olm sessions) before syncing. This does
	// network I/O, so it runs here on the connect goroutine rather than in New
	// (which executes on the UI goroutine during a lazy account switch). On
	// failure, encrypted rooms won't decrypt but unencrypted rooms still work.
	if err := a.client.StartCrypto(ctx); err != nil && ctx.Err() == nil {
		a.reportError(err)
	}
	backoff := time.Second
	for ctx.Err() == nil {
		err := a.client.M.SyncWithContext(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			a.reportError(err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
	}
	return ctx.Err()
}

// onSync reads per-room unread counts and timeline pagination tokens. Matrix
// only includes rooms whose state changed in an incremental sync, so the
// per-channel maps are updated in place (never rebuilt from the delta) and the
// account-wide mention total and per-guild badges are recomputed from the full
// maps — otherwise counts for quiet rooms would be lost after the first delta.
func (a *App) onSync(ctx context.Context, resp *mautrix.RespSync, since string) bool {
	type unreadUpdate struct {
		channel   store.ChannelID
		status    backend.UnreadStatus
		highlight int
	}
	var updates []unreadUpdate

	a.mu.Lock()
	for roomID, jr := range resp.Rooms.Join {
		info := a.rooms[roomID]
		if info == nil {
			continue
		}
		if jr.Timeline.PrevBatch != "" {
			info.prevBatch = jr.Timeline.PrevBatch
		}
		if jr.UnreadNotifications != nil {
			status := backend.Read
			if jr.UnreadNotifications.HighlightCount > 0 {
				status = backend.Mentioned
			} else if jr.UnreadNotifications.NotificationCount > 0 {
				status = backend.Unread
			}
			updates = append(updates, unreadUpdate{
				channel:   info.channelID,
				status:    status,
				highlight: jr.UnreadNotifications.HighlightCount,
			})
		}
	}
	a.mu.Unlock()

	if len(updates) == 0 {
		a.ids.flush()
		return true
	}

	a.ui.Post(func() {
		a.mu.Lock()
		for _, u := range updates {
			if u.status == backend.Read {
				delete(a.channelUnread, u.channel)
				delete(a.channelHighlight, u.channel)
			} else {
				a.channelUnread[u.channel] = u.status
				if u.highlight > 0 {
					a.channelHighlight[u.channel] = u.highlight
				} else {
					delete(a.channelHighlight, u.channel)
				}
			}
		}
		a.recomputeAggregatesLocked()
		a.mu.Unlock()
		if a.onReadStateChange != nil {
			a.onReadStateChange()
		}
	})
	a.ids.flush()
	return true
}

// recomputeAggregatesLocked rebuilds the per-guild badge severities and the
// account-wide mention total from the full per-channel maps. Caller holds a.mu.
func (a *App) recomputeAggregatesLocked() {
	guildUnread := make(map[store.GuildID]backend.UnreadStatus, len(a.guildUnread))
	for channel, status := range a.channelUnread {
		room, ok := a.roomByChannel[channel]
		if !ok {
			continue
		}
		guild := a.guildForLocked(room)
		if status > guildUnread[guild] {
			guildUnread[guild] = status
		}
	}
	a.guildUnread = guildUnread

	total := 0
	for _, h := range a.channelHighlight {
		total += h
	}
	a.mentionTotal = total
}

// --- state handlers ---------------------------------------------------------

func (a *App) onStateCreate(ctx context.Context, evt *event.Event) {
	info := a.roomInfoLocked(evt.RoomID)
	content := evt.Content.AsCreate()
	a.mu.Lock()
	if content != nil && content.Type == event.RoomTypeSpace {
		info.isSpace = true
	}
	a.mu.Unlock()
	a.syncRoomEntry(evt.RoomID)
}

func (a *App) onStateName(ctx context.Context, evt *event.Event) {
	info := a.roomInfoLocked(evt.RoomID)
	if name := evt.Content.AsRoomName(); name != nil {
		a.mu.Lock()
		info.name = name.Name
		a.mu.Unlock()
	}
	a.syncRoomEntry(evt.RoomID)
}

func (a *App) onStateTopic(ctx context.Context, evt *event.Event) {
	info := a.roomInfoLocked(evt.RoomID)
	if t := evt.Content.AsTopic(); t != nil {
		a.mu.Lock()
		info.topic = t.Topic
		a.mu.Unlock()
	}
}

func (a *App) onStateAvatar(ctx context.Context, evt *event.Event) {
	info := a.roomInfoLocked(evt.RoomID)
	if av := evt.Content.AsRoomAvatar(); av != nil {
		if uri, ok := matrix.ParseMXC(av.URL); ok {
			a.mu.Lock()
			info.avatar = matrix.DownloadURL(a.client.Creds.Homeserver, uri)
			a.mu.Unlock()
		}
	}
}

func (a *App) onStateMember(ctx context.Context, evt *event.Event) {
	a.roomInfoLocked(evt.RoomID)
	member := evt.Content.AsMember()
	if member == nil {
		return
	}
	userID := id.UserID(evt.GetStateKey())
	joined := member.Membership == event.MembershipJoin
	isSelf := userID == a.selfID

	// Our own membership drives whether the room is materialized at all: a
	// leave/ban removes it, and only a join renders it as a channel.
	if isSelf {
		a.mu.Lock()
		if info := a.rooms[evt.RoomID]; info != nil {
			info.joined = joined
		}
		a.mu.Unlock()
		if !joined {
			a.removeRoom(evt.RoomID)
			return
		}
	}

	uid := store.UserID(a.ids.intern(string(userID)))
	guild := a.guildFor(evt.RoomID)

	// Non-joined members (left, banned, invited) are removed from the member
	// list rather than shown.
	if !joined {
		a.mu.Lock()
		if names := a.memberNames[evt.RoomID]; names != nil {
			delete(names, userID)
		}
		a.mu.Unlock()
		a.ui.Post(func() { a.store.RemoveMember(guild, uid) })
		return
	}

	name := member.Displayname
	if name == "" {
		name = localpart(userID)
	}
	a.mu.Lock()
	if a.memberNames[evt.RoomID] == nil {
		a.memberNames[evt.RoomID] = map[id.UserID]string{}
	}
	a.memberNames[evt.RoomID][userID] = name
	a.mu.Unlock()

	avatar := ""
	if member.AvatarURL != "" {
		if uri, ok := matrix.ParseMXC(member.AvatarURL); ok {
			avatar = matrix.DownloadURL(a.client.Creds.Homeserver, uri)
		}
	}
	m := store.Member{ID: uid, Name: name, Username: localpart(userID), AvatarURL: avatar}
	a.ui.Post(func() {
		a.store.UpsertMember(guild, m)
		if isSelf {
			a.self = store.Member{ID: uid, Name: name, Username: localpart(userID), AvatarURL: avatar}
			a.publishSnapshot()
		}
	})
	if isSelf {
		// Now that we know we're joined, render the room.
		a.syncRoomEntry(evt.RoomID)
	}
}

func (a *App) onSpaceChild(ctx context.Context, evt *event.Event) {
	space := a.roomInfoLocked(evt.RoomID)
	a.mu.Lock()
	space.isSpace = true
	a.mu.Unlock()
	child := id.RoomID(evt.GetStateKey())
	// A child event with empty content ("via" absent) removes the child.
	present := len(evt.Content.Raw) > 0 && evt.Content.Raw["via"] != nil
	a.mu.Lock()
	if present {
		a.childToSpace[child] = evt.RoomID
		space.children = appendUnique(space.children, string(child))
	} else {
		delete(a.childToSpace, child)
	}
	a.mu.Unlock()
	a.syncRoomEntry(evt.RoomID)
	a.syncRoomEntry(child)
}

func (a *App) onDirectChats(ctx context.Context, evt *event.Event) {
	content := evt.Content.AsDirectChats()
	if content == nil {
		return
	}
	a.mu.Lock()
	a.directRooms = map[id.RoomID]bool{}
	for _, rooms := range *content {
		for _, r := range rooms {
			a.directRooms[r] = true
		}
	}
	rooms := make([]id.RoomID, 0, len(a.rooms))
	for r := range a.rooms {
		rooms = append(rooms, r)
	}
	a.mu.Unlock()
	for _, r := range rooms {
		a.syncRoomEntry(r)
	}
}

// --- timeline handlers ------------------------------------------------------

func (a *App) onMessage(ctx context.Context, evt *event.Event) {
	if evt.Type != event.EventMessage {
		return
	}
	channel := a.channelFor(evt.RoomID) // ensures the room entry exists
	a.syncRoomEntry(evt.RoomID)
	content := evt.Content.AsMessage()
	if content == nil {
		return
	}

	// Edits: m.replace replaces the target's content.
	if content.RelatesTo != nil && content.RelatesTo.Type == event.RelReplace {
		targetID := content.RelatesTo.EventID
		target := store.MessageID(a.ids.event(string(targetID)))
		newBody := ""
		if content.NewContent != nil {
			newBody = content.NewContent.Body
		} else {
			newBody = strings.TrimPrefix(content.Body, "* ")
		}
		a.ui.Post(func() {
			a.store.UpdateMessage(channel, target, func(m *store.Message) { m.Content = newBody })
			a.fireChange()
		})
		return
	}

	msg := a.convertMessage(evt, content, channel)
	fromSelf := evt.Sender == a.selfID
	a.ui.Post(func() {
		// Reconcile our own optimistic echo by nonce first; otherwise dedupe
		// against redelivered events (reconnect / gappy sync re-send timeline
		// events) before appending, mirroring the Discord ingest path.
		if msg.Nonce != "" && a.store.ReplaceMessage(msg.Nonce, msg) {
			a.fireChange()
			return
		}
		if a.store.HasMessage(channel, msg.ID) {
			return // duplicate redelivery, already present
		}
		a.store.AppendMessage(msg)
		a.fireChange()
		if !fromSelf && a.onIncoming != nil {
			a.onIncoming(msg)
		}
	})
}

// onDecryptError renders a placeholder for a message that could not be
// decrypted, so E2EE rooms with missing keys still show that a message exists.
func (a *App) onDecryptError(evt *event.Event, decErr error) {
	channel := a.channelFor(evt.RoomID)
	msg := store.Message{
		ID:        store.MessageID(a.ids.event(string(evt.ID))),
		ChannelID: channel,
		AuthorID:  store.UserID(a.ids.intern(string(evt.Sender))),
		Author:    a.displayName(evt.RoomID, evt.Sender),
		Content:   "🔒 Unable to decrypt this message",
		Timestamp: time.UnixMilli(evt.Timestamp),
	}
	a.ui.Post(func() {
		if a.store.HasMessage(channel, msg.ID) {
			return // duplicate redelivery
		}
		a.store.AppendMessage(msg)
		a.fireChange()
	})
}

func (a *App) onReaction(ctx context.Context, evt *event.Event) {
	content := evt.Content.AsReaction()
	if content == nil {
		return
	}
	rel := content.RelatesTo
	if rel.Type != event.RelAnnotation {
		return
	}
	channel := a.channelFor(evt.RoomID)
	target := store.MessageID(a.ids.event(string(rel.EventID)))
	key := rel.Key
	me := evt.Sender == a.selfID

	a.mu.Lock()
	if _, seen := a.reactions[evt.ID]; seen {
		a.mu.Unlock()
		return // duplicate redelivery: the reaction event is already counted
	}
	a.reactions[evt.ID] = reactionRef{channel: channel, message: target, key: key}
	// Bound the redaction-routing map: evicting the oldest reaction only means a
	// redaction of a very old reaction can't un-react it (cosmetic).
	a.reactionOrder = append(a.reactionOrder, evt.ID)
	if len(a.reactionOrder) > eventIDCap {
		evict := a.reactionOrder[0]
		a.reactionOrder = a.reactionOrder[1:]
		delete(a.reactions, evict)
	}
	a.mu.Unlock()

	a.ui.Post(func() {
		a.store.AddReaction(channel, target, store.Reaction{EmojiName: key, Count: 1, Me: me})
		a.fireChange()
	})
}

func (a *App) onRedaction(ctx context.Context, evt *event.Event) {
	redacts := evt.Redacts
	if redacts == "" {
		return
	}
	a.mu.Lock()
	ref, isReaction := a.reactions[redacts]
	if isReaction {
		delete(a.reactions, redacts)
	}
	a.mu.Unlock()

	if isReaction {
		a.ui.Post(func() {
			a.store.RemoveReaction(ref.channel, ref.message, ref.key, 0, false)
			a.fireChange()
		})
		return
	}
	channel := a.channelFor(evt.RoomID)
	target := store.MessageID(a.ids.event(string(redacts)))
	a.ui.Post(func() {
		a.store.RemoveMessage(channel, target)
		a.fireChange()
	})
}

// --- conversion -------------------------------------------------------------

func (a *App) convertMessage(evt *event.Event, content *event.MessageEventContent, channel store.ChannelID) store.Message {
	body := content.Body
	// Strip the plaintext reply fallback ("> ..." quoted lines).
	if content.RelatesTo.GetReplyTo() != "" {
		body = stripReplyFallback(body)
	}
	msg := store.Message{
		ID:        store.MessageID(a.ids.event(string(evt.ID))),
		ChannelID: channel,
		AuthorID:  store.UserID(a.ids.intern(string(evt.Sender))),
		Author:    a.displayName(evt.RoomID, evt.Sender),
		Content:   body,
		Timestamp: time.UnixMilli(evt.Timestamp),
	}
	if txn := evt.Unsigned.TransactionID; txn != "" && evt.Sender == a.selfID {
		msg.Nonce = txn
	}
	if replyTo := content.RelatesTo.GetReplyTo(); replyTo != "" {
		msg.Reply = &store.MessageReply{
			MessageID: store.MessageID(a.ids.event(string(replyTo))),
			ChannelID: channel,
		}
	}
	if att, ok := a.convertAttachment(content); ok {
		msg.Attachments = []store.Attachment{att}
	}
	return msg
}

func (a *App) convertAttachment(content *event.MessageEventContent) (store.Attachment, bool) {
	switch content.MsgType {
	case event.MsgImage, event.MsgFile, event.MsgVideo, event.MsgAudio:
	default:
		return store.Attachment{}, false
	}
	var url string
	if content.File != nil && content.File.URL != "" {
		if uri, ok := matrix.ParseMXC(content.File.URL); ok {
			url = matrix.DownloadURL(a.client.Creds.Homeserver, uri)
			a.authorizer.registerEncrypted(url, content.File)
		}
	} else if content.URL != "" {
		if uri, ok := matrix.ParseMXC(content.URL); ok {
			url = matrix.DownloadURL(a.client.Creds.Homeserver, uri)
		}
	}
	if url == "" {
		return store.Attachment{}, false
	}
	att := store.Attachment{URL: url, Filename: filenameOf(content)}
	if content.Info != nil {
		att.ContentType = content.Info.MimeType
		att.W = content.Info.Width
		att.H = content.Info.Height
		att.Size = int64(content.Info.Size)
	}
	return att, true
}

// --- helpers ----------------------------------------------------------------

func (a *App) fireChange() {
	if a.onChange != nil {
		a.onChange()
	}
}

func localpart(user id.UserID) string {
	s := string(user)
	s = strings.TrimPrefix(s, "@")
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return s[:i]
	}
	return s
}

func (a *App) displayName(room id.RoomID, user id.UserID) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if names, ok := a.memberNames[room]; ok {
		if n, ok := names[user]; ok && n != "" {
			return n
		}
	}
	return localpart(user)
}

func filenameOf(content *event.MessageEventContent) string {
	if content.FileName != "" {
		return content.FileName
	}
	return content.Body
}

func appendUnique(s []string, v string) []string {
	for _, existing := range s {
		if existing == v {
			return s
		}
	}
	return append(s, v)
}

// stripReplyFallback removes the leading "> " quoted lines mautrix uses as the
// plaintext reply fallback, leaving the actual message body.
func stripReplyFallback(body string) string {
	lines := strings.Split(body, "\n")
	i := 0
	for i < len(lines) && strings.HasPrefix(lines[i], "> ") {
		i++
	}
	// Skip the blank separator line the fallback leaves behind.
	if i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) {
		return body
	}
	return strings.Join(lines[i:], "\n")
}
