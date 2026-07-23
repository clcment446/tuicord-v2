package matrixapp

import (
	"context"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"awesomeProject/internal/backend"
	"awesomeProject/internal/store"
)

// syncPoster runs posted closures immediately, as if already on the UI goroutine.
type syncPoster struct{}

func (syncPoster) Post(fn func())         { fn() }
func (syncPoster) TryPost(fn func()) bool { fn(); return true }
func (syncPoster) WriteRaw([]byte)        {}
func (syncPoster) Invalidate()            {}
func (syncPoster) ForceRepaint()          {}

// newTestApp builds an App without a real Matrix client, for exercising the
// text-message ingest path (which does not touch the transport). Media/avatar
// paths that dereference the client are out of scope here.
func newTestApp() (*App, *store.Store) {
	st := store.New(0)
	a := &App{
		store:            st,
		ui:               syncPoster{},
		ids:              newIDMap(""),
		selfID:           id.UserID("@me:example.org"),
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
	a.self = store.Member{ID: store.UserID(a.ids.intern("@me:example.org"))}
	return a, st
}

func textEvent(room id.RoomID, sender id.UserID, eventID id.EventID, body string) *event.Event {
	return &event.Event{
		Type:      event.EventMessage,
		RoomID:    room,
		Sender:    sender,
		ID:        eventID,
		Timestamp: 1_700_000_000_000,
		Content:   event.Content{Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: body}},
	}
}

func TestOnMessageDedupesRedeliveredEvents(t *testing.T) {
	a, st := newTestApp()
	room := id.RoomID("!room:example.org")
	alice := id.UserID("@alice:example.org")
	evt := textEvent(room, alice, "$evt1", "hello")

	a.onMessage(context.Background(), evt)
	a.onMessage(context.Background(), evt) // redelivery (reconnect / gappy sync)

	channel := a.channelFor(room)
	if got := len(st.Messages(channel)); got != 1 {
		t.Fatalf("redelivered event produced %d messages, want 1", got)
	}

	a.onMessage(context.Background(), textEvent(room, alice, "$evt2", "world"))
	if got := len(st.Messages(channel)); got != 2 {
		t.Fatalf("distinct event count = %d, want 2", got)
	}
}

func TestOnMessageReconcilesOwnEchoByNonce(t *testing.T) {
	a, st := newTestApp()
	room := id.RoomID("!room:example.org")
	channel := a.channelFor(room)

	// Optimistic local echo, as SendToChannel would append.
	a.store.AppendMessage(store.Message{ChannelID: channel, Content: "hi", Nonce: "txn-1", Pending: true})

	// The server echoes it back with our transaction id in unsigned.
	echo := textEvent(room, a.selfID, "$echo", "hi")
	echo.Unsigned.TransactionID = "txn-1"
	a.onMessage(context.Background(), echo)

	msgs := st.Messages(channel)
	if len(msgs) != 1 {
		t.Fatalf("own echo produced %d messages, want 1 (reconciled)", len(msgs))
	}
	if msgs[0].Pending {
		t.Fatalf("reconciled message is still pending")
	}
}

func memberEvent(room id.RoomID, user id.UserID, membership event.Membership, name string) *event.Event {
	stateKey := string(user)
	return &event.Event{
		Type:     event.StateMember,
		RoomID:   room,
		Sender:   user,
		StateKey: &stateKey,
		Content:  event.Content{Parsed: &event.MemberEventContent{Membership: membership, Displayname: name}},
	}
}

func TestRoomRenderedOnlyWhenJoinedAndRemovedOnLeave(t *testing.T) {
	a, st := newTestApp()
	room := id.RoomID("!room:example.org")
	channel := a.channelFor(room)

	// Before our own join, the room must not appear as a channel.
	a.syncRoomEntry(room)
	if _, ok := st.Channel(channel); ok {
		t.Fatal("room rendered before we joined it")
	}

	// Our own join materializes it.
	a.onStateMember(context.Background(), memberEvent(room, a.selfID, event.MembershipJoin, "me"))
	if _, ok := st.Channel(channel); !ok {
		t.Fatal("joined room was not rendered")
	}

	// Leaving removes it.
	a.onStateMember(context.Background(), memberEvent(room, a.selfID, event.MembershipLeave, "me"))
	if _, ok := st.Channel(channel); ok {
		t.Fatal("left room was not removed")
	}
}

func TestMemberListFiltersByMembership(t *testing.T) {
	a, st := newTestApp()
	room := id.RoomID("!room:example.org")
	a.onStateMember(context.Background(), memberEvent(room, a.selfID, event.MembershipJoin, "me"))
	guild := a.guildFor(room)
	alice := id.UserID("@alice:example.org")

	a.onStateMember(context.Background(), memberEvent(room, alice, event.MembershipJoin, "Alice"))
	if _, ok := st.Member(guild, store.UserID(a.ids.intern(string(alice)))); !ok {
		t.Fatal("joined member not added")
	}
	a.onStateMember(context.Background(), memberEvent(room, alice, event.MembershipLeave, "Alice"))
	if _, ok := st.Member(guild, store.UserID(a.ids.intern(string(alice)))); ok {
		t.Fatal("left member not removed from the list")
	}
}

func TestUnreadAggregatesSurviveIncrementalSyncs(t *testing.T) {
	a, _ := newTestApp()
	roomA := id.RoomID("!a:example.org")
	roomB := id.RoomID("!b:example.org")
	// Put the two rooms in distinct guilds: A in the ungrouped "Rooms" guild, B
	// as a DM. (Two ungrouped rooms would correctly share one guild badge.)
	a.directRooms[roomB] = true
	chanA := a.channelFor(roomA)
	chanB := a.channelFor(roomB)

	// Simulate two rooms accumulating unread across separate syncs.
	a.mu.Lock()
	a.channelUnread[chanA] = backend.Mentioned
	a.channelHighlight[chanA] = 2
	a.recomputeAggregatesLocked()
	a.mu.Unlock()

	a.mu.Lock()
	a.channelUnread[chanB] = backend.Unread
	a.recomputeAggregatesLocked()
	a.mu.Unlock()

	// Room A's mention must not be lost when room B's later sync arrives.
	if unread, mentions := a.Unread(); !unread || mentions != 2 {
		t.Fatalf("Unread() = (%v, %d), want (true, 2)", unread, mentions)
	}
	if got := a.GuildUnread(a.guildFor(roomA)); got != backend.Mentioned {
		t.Fatalf("guild A unread = %v, want Mentioned", got)
	}
	if got := a.GuildUnread(a.guildFor(roomB)); got != backend.Unread {
		t.Fatalf("guild B unread = %v, want Unread", got)
	}

	// Opening room A clears its mention; room B's unread remains.
	a.SetActive(a.guildFor(roomA), chanA)
	if _, mentions := a.Unread(); mentions != 0 {
		t.Fatalf("mentions after opening A = %d, want 0", mentions)
	}
	if got := a.GuildUnread(a.guildFor(roomB)); got != backend.Unread {
		t.Fatalf("guild B unread after opening A = %v, want Unread", got)
	}
}
