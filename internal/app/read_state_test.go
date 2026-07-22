package app

import (
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

	// Seed one effective mention and one authoritative read position. These are
	// already in ningen when App receives READY in production.
	ning.ReadState.MarkRead(42, 50)
	ning.ReadState.MarkUnread(42, 60, 1)
	ning.ReadState.MarkRead(43, 70)
	st.IncrementUnread(43)
	st.IncrementPing(43)
	a.cacheReadState(99, 8, Mentioned)

	a.handleReady(&gateway.ReadyEvent{})

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

func TestReadUpdateUsesEffectiveStateAndClearsLocalPing(t *testing.T) {
	ning := appdiscord.WrapSession(session.New(""))
	st := store.New(0)
	st.UpsertGuild(store.Guild{ID: 7})
	st.UpsertChannel(store.Channel{ID: 42, GuildID: 7})
	st.IncrementUnread(42)
	st.IncrementPing(42)
	a := &App{store: st, ui: syncPoster{}, handle: ning}

	// The raw event says unread, but ningen has a valid read position and no
	// visible channel in its cabinet, so effective permission/latest semantics
	// classify it as read.
	ning.ReadState.MarkRead(42, 50)
	a.handleReadStateUpdate(&read.UpdateEvent{
		ReadState: gateway.ReadState{ChannelID: 42, LastMessageID: 50},
		GuildID:   7,
		Unread:    true,
	})

	if got := a.GuildUnread(7); got != Read {
		t.Fatalf("effective guild status = %v, want read", got)
	}
	if st.Unread(42) != 0 || st.Pings(42) != 0 || st.GuildPings(7) != 0 {
		t.Fatalf("acknowledged fallback survived: unread=%d pings=%d guild=%d", st.Unread(42), st.Pings(42), st.GuildPings(7))
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
			ning.ReadState.MarkRead(42, tt.read)
			a := &App{store: store.New(0), handle: ning}

			a.MarkRead(42, 100)

			state := ning.ReadState.ReadState(42)
			if state == nil || state.LastMessageID != tt.want {
				t.Fatalf("read position = %+v, want %d", state, tt.want)
			}
		})
	}
}
