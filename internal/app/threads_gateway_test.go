package app

import (
	"testing"

	"awesomeProject/internal/store"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
)

func TestHandleThreadUpsertPreservesJoined(t *testing.T) {
	a := &App{store: store.New(0), ui: syncPoster{}}
	a.store.UpsertChannel(store.Channel{ID: 100, GuildID: 1, Kind: store.ChannelText})
	// First: an update carrying our membership.
	joinedCh := discord.Channel{ID: 10, Type: discord.GuildPublicThread, ParentID: 100,
		ThreadMember: &discord.ThreadMember{}, Name: "t"}
	a.handleThreadUpsert(joinedCh)
	c, _ := a.store.Channel(10)
	if !c.Thread.Joined {
		t.Fatal("thread should be joined after create with ThreadMember")
	}
	// Second: a plain update (no ThreadMember) must not clear Joined.
	a.handleThreadUpsert(discord.Channel{ID: 10, Type: discord.GuildPublicThread, ParentID: 100, Name: "t2"})
	c, _ = a.store.Channel(10)
	if !c.Thread.Joined {
		t.Error("plain update should preserve prior Joined state")
	}
	if c.Name != "t2" {
		t.Errorf("name = %q, want updated t2", c.Name)
	}
}

func TestHandleThreadListSync(t *testing.T) {
	a := &App{store: store.New(0), ui: syncPoster{}}
	e := &gateway.ThreadListSyncEvent{
		GuildID: 1,
		Threads: []discord.Channel{
			{ID: 10, Type: discord.GuildPublicThread, ParentID: 100, Name: "a"},
			{ID: 11, Type: discord.GuildPublicThread, ParentID: 100, Name: "b"},
		},
		Members: []discord.ThreadMember{{ID: 11}},
	}
	a.handleThreadListSync(e)
	if len(a.store.Threads(100)) != 2 {
		t.Fatalf("expected 2 active threads, got %d", len(a.store.Threads(100)))
	}
	c, _ := a.store.Channel(11)
	if !c.Thread.Joined {
		t.Error("thread 11 should be joined from sync Members")
	}
}

func TestHandleThreadMembersUpdateOwnJoinLeave(t *testing.T) {
	a := &App{store: store.New(0), ui: syncPoster{}, selfID: 42}
	a.store.UpsertChannel(store.Channel{ID: 100, GuildID: 1, Kind: store.ChannelText})
	a.store.UpsertThread(store.Channel{ID: 10, GuildID: 1, ParentID: 100, Thread: &store.ThreadMeta{}})

	a.handleThreadMembersUpdate(&gateway.ThreadMembersUpdateEvent{
		ID: 10, AddedMembers: []discord.ThreadMember{{UserID: 42}},
	})
	c, _ := a.store.Channel(10)
	if !c.Thread.Joined {
		t.Fatal("should be joined after being added")
	}
	a.handleThreadMembersUpdate(&gateway.ThreadMembersUpdateEvent{
		ID: 10, RemovedMemberIDs: []discord.UserID{42},
	})
	c, _ = a.store.Channel(10)
	if c.Thread.Joined {
		t.Error("should not be joined after being removed")
	}
}

func TestHandleThreadDeleteRepairsActiveSelection(t *testing.T) {
	a := &App{store: store.New(0), ui: syncPoster{}}
	a.store.UpsertChannel(store.Channel{ID: 100, GuildID: 1, Kind: store.ChannelText})
	a.store.UpsertThread(store.Channel{
		ID: 10, GuildID: 1, ParentID: 100, Thread: &store.ThreadMeta{},
	})
	a.store.AppendMessage(store.Message{ID: 1, ChannelID: 10})
	a.SetActive(1, 10)

	a.handleThreadDelete(&gateway.ThreadDeleteEvent{ID: 10, GuildID: 1, ParentID: 100})

	if _, ok := a.store.Channel(10); ok || a.store.Messages(10) != nil {
		t.Fatal("deleted active thread or its history remains cached")
	}
	if a.ActiveGuild() != 1 || a.ActiveChannel() != 0 {
		t.Fatalf("active selection = %d/%d, want 1/0", a.ActiveGuild(), a.ActiveChannel())
	}
}

func TestHandleThreadMembersUpdateIgnoresOthers(t *testing.T) {
	a := &App{store: store.New(0), ui: syncPoster{}, selfID: 42}
	a.store.UpsertChannel(store.Channel{ID: 100, GuildID: 1, Kind: store.ChannelText})
	a.store.UpsertThread(store.Channel{ID: 10, GuildID: 1, ParentID: 100, Thread: &store.ThreadMeta{Joined: true}})
	// Another user leaving must not touch our membership.
	a.handleThreadMembersUpdate(&gateway.ThreadMembersUpdateEvent{
		ID: 10, RemovedMemberIDs: []discord.UserID{999},
	})
	c, _ := a.store.Channel(10)
	if !c.Thread.Joined {
		t.Error("another user's leave should not clear our Joined")
	}
}
