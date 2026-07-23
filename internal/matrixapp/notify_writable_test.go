package matrixapp

import (
	"context"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"awesomeProject/internal/store"
)

// TestInitialSyncBacklogDoesNotNotify guards the notification-flood fix: the
// initial sync replays each room's recent backlog as timeline events, and those
// must not raise OnIncomingMessage. Only live messages (after the client is
// caught up) notify, and never the account's own messages.
func TestInitialSyncBacklogDoesNotNotify(t *testing.T) {
	a, _ := newTestApp()
	var notified int
	a.onIncoming = func(store.Message) { notified++ }
	room := id.RoomID("!room:example.org")
	alice := id.UserID("@alice:example.org")

	// Initial sync (caughtUp false): backlog lands silently.
	a.onMessage(context.Background(), textEvent(room, alice, "$b1", "old1"))
	a.onMessage(context.Background(), textEvent(room, alice, "$b2", "old2"))
	if notified != 0 {
		t.Fatalf("initial-sync backlog fired %d notifications, want 0", notified)
	}

	// Live stream: notify.
	a.caughtUp.Store(true)
	a.onMessage(context.Background(), textEvent(room, alice, "$live", "new"))
	if notified != 1 {
		t.Fatalf("live message fired %d notifications, want 1", notified)
	}

	// Our own live echo never notifies.
	a.onMessage(context.Background(), textEvent(room, a.selfID, "$echo", "self"))
	if notified != 1 {
		t.Fatalf("self message raised a notification (total %d), want it suppressed", notified)
	}
}

// TestOnSyncTracksLiveGate checks that the live-stream gate follows the sync
// token: empty on the initial (or from-scratch reconnect) sync, set once the
// client is following the live stream.
func TestOnSyncTracksLiveGate(t *testing.T) {
	a, _ := newTestApp()
	a.onSync(context.Background(), &mautrix.RespSync{}, "")
	if a.caughtUp.Load() {
		t.Fatal("initial sync (empty token) should leave caughtUp false")
	}
	a.onSync(context.Background(), &mautrix.RespSync{}, "s_batch_1")
	if !a.caughtUp.Load() {
		t.Fatal("live sync (non-empty token) should set caughtUp true")
	}
	// A from-scratch reconnect (empty token again) suppresses its catch-up batch.
	a.onSync(context.Background(), &mautrix.RespSync{}, "")
	if a.caughtUp.Load() {
		t.Fatal("reconnect from scratch should reset caughtUp to false")
	}
}

// TestMatrixGuildIsWritable guards the read-only-composer fix: a Matrix room in
// a synthetic guild must be writable. The client's read-only gate is the store
// permission model, which denies everything for a guild with no @everyone role.
func TestMatrixGuildIsWritable(t *testing.T) {
	a, st := newTestApp()
	for _, guild := range []store.GuildID{UngroupedRoomsGuildID, store.GuildID(42)} {
		a.ensureGuildRow(guild)
		channel := store.ChannelID(uint64(guild) ^ 0x1234)
		st.UpsertChannel(store.Channel{ID: channel, GuildID: guild, Kind: store.ChannelText})
		if !st.ChannelCan(guild, a.self.ID, channel, store.PermSendMessages) {
			t.Fatalf("guild %d: room should allow sending messages", guild)
		}
		// Participation only — management stays denied so Discord moderation UI
		// does not appear on Matrix rooms.
		if st.ChannelCan(guild, a.self.ID, channel, store.PermManageChannels) {
			t.Fatalf("guild %d: should not grant management permissions", guild)
		}
	}
}
