package app

import (
	"sync"
	"testing"

	appdiscord "awesomeProject/internal/discord"
	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/ningen/v3/states/read"
)

func TestUnreadStatusFallsBackUntilServerReadStateExists(t *testing.T) {
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 7, Name: "Home"})
	st.UpsertChannel(store.Channel{ID: 42, GuildID: 7, Name: "general"})
	a := &App{store: st}

	st.IncrementUnread(42)
	if got := a.ChannelUnread(42); got != Unread {
		t.Fatalf("channel status = %v, want unread", got)
	}
	st.IncrementPing(42)
	a.cacheReadState(42, 7, Mentioned)
	if got := a.GuildUnread(7); got != Mentioned {
		t.Fatalf("guild status = %v, want mentioned", got)
	}
}

func TestMarkChannelReadClearsLocalFallback(t *testing.T) {
	st := store.New(0)
	st.IncrementUnread(42)
	st.IncrementPing(42)
	a := &App{store: st}

	a.MarkChannelRead(42)

	if got := st.Unread(42); got != 0 || st.Pings(42) != 0 {
		t.Fatalf("local read state = unread %d, pings %d; want both zero", got, st.Pings(42))
	}
}

func TestGuildUnreadUsesCachedReadState(t *testing.T) {
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 7, Name: "Home"})
	st.UpsertChannel(store.Channel{ID: 42, GuildID: 7, Name: "general"})
	a := &App{store: st}

	a.cacheReadState(42, 7, Mentioned)
	if got := a.GuildUnread(7); got != Mentioned {
		t.Fatalf("cached guild status = %v, want mentioned", got)
	}

	a.cacheReadState(42, 7, Read)
	if got := a.GuildUnread(7); got != Read {
		t.Fatalf("cached cleared guild status = %v, want read", got)
	}
}

func TestCachedReadStateKeepsOtherGuildChannels(t *testing.T) {
	st := store.New(0)
	a := &App{store: st}

	a.cacheReadState(41, 7, Unread)
	a.cacheReadState(42, 7, Mentioned)
	a.cacheReadState(42, 7, Read)

	if got := a.GuildUnread(7); got != Unread {
		t.Fatalf("remaining channel status = %v, want unread", got)
	}
}

func TestReadySeedsAndResetsReadStateCache(t *testing.T) {
	ning := appdiscord.WrapSession(session.New(""))
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 7, Name: "current"})
	st.UpsertChannel(store.Channel{ID: 42, GuildID: 7, Name: "mentioned"})
	st.UpsertChannel(store.Channel{ID: 43, GuildID: 7, Name: "acked"})
	st.UpsertGuild(store.Guild{ID: 8, Name: "stale"})
	st.UpsertChannel(store.Channel{ID: 99, GuildID: 8, Name: "old"})
	a := &App{store: st, ui: syncPoster{}, handle: ning}

	// READY carries the connection's read markers: one effective mention and one
	// authoritative read position. App snapshots these value copies instead of
	// reading ningen's live pointers.
	st.IncrementUnread(43)
	st.IncrementPing(43)
	a.cacheReadState(99, 8, Mentioned)

	ready := &gateway.ReadyEvent{}
	ready.ReadStates = []gateway.ReadState{
		{ChannelID: 42, LastMessageID: 60, MentionCount: 1},
		{ChannelID: 43, LastMessageID: 70},
	}
	a.handleReady(ready)

	if got := a.GuildUnread(7); got != Mentioned {
		t.Fatalf("READY-seeded guild status = %v, want mentioned", got)
	}
	if got := a.GuildUnread(8); got != Read {
		t.Fatalf("stale reconnect status = %v, want read", got)
	}
	if st.Unread(43) != 0 || st.Pings(43) != 0 {
		t.Fatalf("authoritatively read local fallback survived READY: unread=%d pings=%d", st.Unread(43), st.Pings(43))
	}
}

func TestChannelUnreadDispatchBatchesBeforeNingenHydration(t *testing.T) {
	ning := appdiscord.WrapSession(session.New(""))
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 7})
	changes := 0
	a := &App{store: st, ui: syncPoster{}, handle: ning, onReadStateChange: func() { changes++ }}
	event := &gateway.ChannelUnreadUpdateEvent{GuildID: 7}
	for i := 0; i < 500; i++ {
		id := discord.ChannelID(42 + i)
		st.UpsertChannel(store.Channel{ID: store.ChannelID(id), GuildID: 7})
		event.ChannelUnreadUpdates = append(event.ChannelUnreadUpdates, struct {
			ID            discord.ChannelID `json:"id"`
			LastMessageID discord.MessageID `json:"last_message_id"`
		}{ID: id, LastMessageID: discord.MessageID(1000 + i)})
	}

	a.handleChannelUnreadUpdate(event)

	if got := a.ChannelUnread(42); got != Unread {
		t.Fatalf("channel status = %v, want unread", got)
	}
	if got := a.GuildUnread(7); got != Unread {
		t.Fatalf("guild status = %v, want unread", got)
	}
	if changes != 1 {
		t.Fatalf("read-state callbacks = %d, want one batched callback", changes)
	}
	if state := ning.ReadState.ReadState(42); state != nil {
		t.Fatalf("bulk dispatch was mirrored through ningen MarkUnread: %+v", state)
	}
}

func TestMuteLookupDoesNotReadUIOwnedStore(t *testing.T) {
	ning := appdiscord.WrapSession(session.New(""))
	ning.ChannelSet(&discord.Channel{ID: 42, GuildID: 7, Type: discord.GuildText}, true)
	st := store.New(0)
	a := New(ning, st, nil)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			st.UpsertChannel(store.Channel{ID: 42, GuildID: 7, Position: i})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = a.channelMutedLocal(42)
		}
	}()
	wg.Wait()
}

func TestReadUpdateToleratesMissingStore(t *testing.T) {
	ning := appdiscord.WrapSession(session.New(""))
	a := &App{ui: syncPoster{}, handle: ning}

	a.handleReadStateUpdate(&read.UpdateEvent{
		ReadState: gateway.ReadState{ChannelID: 42, LastMessageID: 50},
		GuildID:   7,
		Unread:    true,
	})

	if got := a.GuildUnread(7); got != Unread {
		t.Fatalf("cached status = %v, want unread", got)
	}
}

func TestNewInitializesMissingStore(t *testing.T) {
	a := New(appdiscord.WrapSession(session.New("")), nil, nil)
	if a.Store() == nil {
		t.Fatal("New retained a nil store")
	}
}

func TestReadAcknowledgementClearsLocalPing(t *testing.T) {
	ning := appdiscord.WrapSession(session.New(""))
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 7})
	st.UpsertChannel(store.Channel{ID: 42, GuildID: 7})
	st.IncrementUnread(42)
	st.IncrementPing(42)
	a := &App{store: st, ui: syncPoster{}, handle: ning}

	ning.ReadState.MarkRead(42, 50)
	a.handleReadStateUpdate(&read.UpdateEvent{
		ReadState: gateway.ReadState{ChannelID: 42, LastMessageID: 50},
		GuildID:   7,
		Unread:    false,
	})

	if got := a.GuildUnread(7); got != Read {
		t.Fatalf("effective guild status = %v, want read", got)
	}
	if st.Unread(42) != 0 || st.Pings(42) != 0 || st.GuildPings(7) != 0 {
		t.Fatalf("acknowledged fallback survived: unread=%d pings=%d guild=%d", st.Unread(42), st.Pings(42), st.GuildPings(7))
	}
}

func TestMuteSettingChangeRefreshesCachedBadges(t *testing.T) {
	ning := appdiscord.WrapSession(session.New(""))
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 7})
	st.UpsertChannel(store.Channel{ID: 42, GuildID: 7, LastMessageID: 100})
	changes := 0
	a := &App{store: st, ui: syncPoster{}, handle: ning, onReadStateChange: func() { changes++ }}

	// Channel 42 is unread (read watermark behind the latest message).
	a.putReadMark(42, readMark{lastRead: 50})
	a.resetReadStateCache()
	if got := a.GuildUnread(7); got != Unread {
		t.Fatalf("pre-mute status = %v, want unread", got)
	}

	// Mute the channel through ningen, then deliver the settings update to App.
	event := &gateway.UserGuildSettingsUpdateEvent{UserGuildSetting: gateway.UserGuildSetting{
		GuildID:          7,
		ChannelOverrides: []gateway.UserChannelOverride{{ChannelID: 42, Muted: true}},
	}}
	// ningen registers its MutedState on the embedded state handler (the
	// prehandler), so dispatch there. READY initializes the mute maps; the
	// settings update then applies the mute before App recomputes.
	ning.State.Handler.Call(&gateway.ReadyEvent{})
	ning.State.Handler.Call(event)
	a.handleGuildSettingsUpdate(event)

	if got := a.GuildUnread(7); got != Read {
		t.Fatalf("muted channel status = %v, want read", got)
	}
	if changes == 0 {
		t.Fatal("mute change did not fire a read-state refresh")
	}
}

func TestDMAckWithZeroGuildClearsSyntheticDMBadge(t *testing.T) {
	ning := appdiscord.WrapSession(session.New(""))
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: store.GuildID(DirectMessagesGuildID), Name: "Direct Messages"})
	st.UpsertChannel(store.Channel{ID: 42, GuildID: DirectMessagesGuildID, Kind: store.ChannelDM})
	st.IncrementUnread(42)
	a := &App{store: st, ui: syncPoster{}, handle: ning}

	// A DM is first observed as unread under the synthetic guild.
	a.cacheReadState(42, DirectMessagesGuildID, Unread)
	if got := a.GuildUnread(DirectMessagesGuildID); got != Unread {
		t.Fatalf("DM guild status = %v, want unread", got)
	}

	// The ack for a DM carries guild id 0; it must still clear the DM badge.
	a.handleReadStateUpdate(&read.UpdateEvent{
		ReadState: gateway.ReadState{ChannelID: 42, LastMessageID: 50},
		GuildID:   0,
		Unread:    false,
	})

	if got := a.GuildUnread(DirectMessagesGuildID); got != Read {
		t.Fatalf("DM guild status after ack = %v, want read", got)
	}
	if st.Unread(42) != 0 {
		t.Fatalf("DM local unread survived ack: %d", st.Unread(42))
	}
}

func TestReadAckAfterReadySurvivesReplaceReadMarks(t *testing.T) {
	ui := &queuedPoster{}
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 7})
	st.UpsertChannel(store.Channel{ID: 42, GuildID: 7})
	a := &App{store: st, ui: ui}

	// READY snapshots the connection's markers and enqueues replaceReadMarks.
	ready := &gateway.ReadyEvent{}
	ready.ReadStates = []gateway.ReadState{{ChannelID: 42, LastMessageID: 60}}
	a.handleReady(ready)
	// A live read ack for the same channel lands before READY's Post drains.
	a.handleReadStateUpdate(&read.UpdateEvent{
		ReadState: gateway.ReadState{ChannelID: 42, LastMessageID: 90},
		GuildID:   7,
	})
	// FIFO drain runs replaceReadMarks (from READY) before the ack's putReadMark, so
	// the ack's newer watermark must survive rather than being wiped as a lost ack.
	ui.run()

	mark, ok := a.lookupReadMark(42)
	if !ok || mark.lastRead != 90 {
		t.Fatalf("read mark = %+v ok=%v, want lastRead 90 (the ack must survive READY)", mark, ok)
	}
}

func TestLateReadAckDoesNotSynchronouslyResurrectDeletedMark(t *testing.T) {
	ui := &queuedPoster{}
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 7})
	st.UpsertChannel(store.Channel{ID: 42, GuildID: 7})
	a := &App{store: st, ui: ui}
	a.putReadMark(42, readMark{lastRead: 50})

	// The channel is deleted; draining its Post drops the cached mark.
	a.handleChannelDelete(&gateway.ChannelDeleteEvent{Channel: discord.Channel{ID: 42, GuildID: 7}})
	ui.run()
	if _, ok := a.lookupReadMark(42); ok {
		t.Fatal("channel delete did not drop the read mark")
	}

	// A read ack for the just-deleted channel arrives late. Because putReadMark now
	// runs on the UI queue rather than synchronously on the ingest goroutine, it
	// cannot inject an orphaned mark ahead of the ordered delete cleanup.
	a.handleReadStateUpdate(&read.UpdateEvent{
		ReadState: gateway.ReadState{ChannelID: 42, LastMessageID: 90},
		GuildID:   7,
	})
	if _, ok := a.lookupReadMark(42); ok {
		t.Fatal("late read ack synchronously resurrected a mark for the deleted channel")
	}
}

func TestDMAckForUnhydratedChannelResolvesToDMGuild(t *testing.T) {
	// The DM channel is not yet stored (still hydrating) when its ack arrives.
	a := &App{store: store.New(0), ui: syncPoster{}}

	// A DM ack carries guild id 0; with no store entry to resolve it, the update
	// must still be routed to the synthetic DM guild rather than dropped.
	a.handleReadStateUpdate(&read.UpdateEvent{
		ReadState: gateway.ReadState{ChannelID: 42, LastMessageID: 50, MentionCount: 1},
		GuildID:   0,
	})

	if got := a.GuildUnread(DirectMessagesGuildID); got != Mentioned {
		t.Fatalf("unhydrated DM ack guild status = %v, want mentioned under the DM guild", got)
	}
	if got := a.ChannelUnread(42); got != Mentioned {
		t.Fatalf("unhydrated DM channel status = %v, want mentioned", got)
	}
}

func TestReadStateCacheRemovedByDeletionPaths(t *testing.T) {
	t.Run("channel and child thread", func(t *testing.T) {
		a := newTestApp(&fakeSender{})
		a.store.UpsertGuild(store.Guild{ID: 7})
		a.store.UpsertChannel(store.Channel{ID: 40, GuildID: 7})
		a.store.UpsertThread(store.Channel{ID: 41, GuildID: 7, ParentID: 40})
		a.store.IncrementPing(41)
		a.cacheReadState(40, 7, Unread)
		a.cacheReadState(41, 7, Mentioned)

		a.handleChannelDelete(&gateway.ChannelDeleteEvent{Channel: discord.Channel{ID: 40, GuildID: 7}})

		if got := a.GuildUnread(7); got != Read || a.store.GuildPings(7) != 0 {
			t.Fatalf("status after parent delete = %v, local guild pings = %d; want read/0", got, a.store.GuildPings(7))
		}
	})

	t.Run("category keeps surviving child channel unread", func(t *testing.T) {
		a := newTestApp(&fakeSender{})
		a.store.UpsertGuild(store.Guild{ID: 7})
		a.store.UpsertChannel(store.Channel{ID: 50, GuildID: 7, Kind: store.ChannelCategory})
		a.store.UpsertChannel(store.Channel{ID: 51, GuildID: 7, ParentID: 50, Kind: store.ChannelText})
		a.cacheReadState(51, 7, Mentioned)

		// Deleting a category does not delete its child channels (Discord re-parents
		// them); their unread badge must survive.
		a.handleChannelDelete(&gateway.ChannelDeleteEvent{Channel: discord.Channel{ID: 50, GuildID: 7}})

		if _, ok := a.store.Channel(51); !ok {
			t.Fatal("child channel was removed with its category")
		}
		if got := a.GuildUnread(7); got != Mentioned {
			t.Fatalf("child unread after category delete = %v, want mentioned", got)
		}
	})

	t.Run("thread", func(t *testing.T) {
		a := newTestApp(&fakeSender{})
		a.store.UpsertGuild(store.Guild{ID: 7})
		a.store.UpsertThread(store.Channel{ID: 41, GuildID: 7, ParentID: 40})
		a.cacheReadState(41, 7, Mentioned)

		a.handleThreadDelete(&gateway.ThreadDeleteEvent{ID: 41, GuildID: 7, ParentID: 40})

		if got := a.GuildUnread(7); got != Read {
			t.Fatalf("status after thread delete = %v, want read", got)
		}
	})

	t.Run("guild", func(t *testing.T) {
		a := newTestApp(&fakeSender{})
		a.store.UpsertGuild(store.Guild{ID: 7})
		a.store.UpsertChannel(store.Channel{ID: 40, GuildID: 7})
		a.cacheReadState(40, 7, Mentioned)

		a.handleGuildDelete(&gateway.GuildDeleteEvent{ID: 7})

		if got := a.GuildUnread(7); got != Read {
			t.Fatalf("status after guild delete = %v, want read", got)
		}
	})
}

func TestMarkReadNeverMovesBehindAuthoritativeStateOrLatestChannel(t *testing.T) {
	tests := []struct {
		name   string
		read   discord.MessageID
		latest discord.MessageID
		want   discord.MessageID
	}{
		{name: "read position", read: 300, latest: 200, want: 300},
		{name: "channel latest", read: 200, latest: 300, want: 300},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ning := appdiscord.WrapSession(session.New(""))
			ning.ChannelSet(&discord.Channel{ID: 42, GuildID: 7, LastMessageID: tt.latest}, true)
			a := &App{store: store.New(0), handle: ning}
			// The read watermark App must not fall behind comes from its own marker
			// snapshot, fed in production by READY and read.UpdateEvent value copies.
			a.putReadMark(42, readMark{lastRead: tt.read})

			a.MarkRead(42, 100)

			state := ning.ReadState.ReadState(42)
			if state == nil || state.LastMessageID != tt.want {
				t.Fatalf("read position = %+v, want %d", state, tt.want)
			}
		})
	}
}
