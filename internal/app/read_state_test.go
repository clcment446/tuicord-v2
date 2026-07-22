package app

import (
	"testing"

	"awesomeProject/internal/store"
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
