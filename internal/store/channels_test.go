package store

import (
	"testing"
	"time"
)

func thread(id ChannelID, parent ChannelID, active time.Time, archived bool) Channel {
	return Channel{
		ID: id, GuildID: 1, Kind: ChannelThread, ParentID: parent,
		Thread: &ThreadMeta{LastActive: active, Archived: archived},
	}
}

func TestThreadsSortedByActivityDesc(t *testing.T) {
	s := New(0)
	s.UpsertChannel(Channel{ID: 100, GuildID: 1, Kind: ChannelText})
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s.UpsertThread(thread(10, 100, base.Add(1*time.Hour), false))
	s.UpsertThread(thread(11, 100, base.Add(3*time.Hour), false))
	s.UpsertThread(thread(12, 100, base.Add(2*time.Hour), false))
	s.UpsertThread(thread(13, 100, base, true)) // archived: excluded

	got := s.Threads(100)
	if len(got) != 3 {
		t.Fatalf("Threads len = %d, want 3 (archived excluded)", len(got))
	}
	wantOrder := []ChannelID{11, 12, 10}
	for i, w := range wantOrder {
		if got[i].ID != w {
			t.Errorf("Threads[%d].ID = %d, want %d", i, got[i].ID, w)
		}
	}
}

func TestThreadsTieBreakByDescendingID(t *testing.T) {
	s := New(0)
	s.UpsertChannel(Channel{ID: 100, GuildID: 1, Kind: ChannelText})
	when := time.Unix(1000, 0)
	s.UpsertThread(thread(10, 100, when, false))
	s.UpsertThread(thread(20, 100, when, false))
	got := s.Threads(100)
	if len(got) != 2 || got[0].ID != 20 || got[1].ID != 10 {
		t.Fatalf("tie-break order = %v, want [20 10]", ids(got))
	}
}

func TestThreadsFilterByParent(t *testing.T) {
	s := New(0)
	s.UpsertChannel(Channel{ID: 100, GuildID: 1, Kind: ChannelText})
	s.UpsertChannel(Channel{ID: 200, GuildID: 1, Kind: ChannelText})
	s.UpsertThread(thread(10, 100, time.Unix(1, 0), false))
	s.UpsertThread(thread(11, 200, time.Unix(1, 0), false))
	if got := s.Threads(100); len(got) != 1 || got[0].ID != 10 {
		t.Errorf("Threads(100) = %v, want [10]", ids(got))
	}
	if got := s.Threads(200); len(got) != 1 || got[0].ID != 11 {
		t.Errorf("Threads(200) = %v, want [11]", ids(got))
	}
}

func TestSetArchivedTransitions(t *testing.T) {
	s := New(0)
	s.UpsertChannel(Channel{ID: 100, GuildID: 1, Kind: ChannelText})
	s.UpsertThread(thread(10, 100, time.Unix(1, 0), false))

	if len(s.Threads(100)) != 1 || len(s.ArchivedThreads(100)) != 0 {
		t.Fatal("precondition: one active thread")
	}
	if !s.SetArchived(10, true) {
		t.Fatal("SetArchived(true) returned false for known thread")
	}
	if len(s.Threads(100)) != 0 || len(s.ArchivedThreads(100)) != 1 {
		t.Errorf("after archive: active=%d archived=%d, want 0/1",
			len(s.Threads(100)), len(s.ArchivedThreads(100)))
	}
	if !s.SetArchived(10, false) {
		t.Fatal("SetArchived(false) returned false")
	}
	if len(s.Threads(100)) != 1 {
		t.Errorf("after unarchive: active=%d, want 1", len(s.Threads(100)))
	}
}

func TestSetArchivedUnknownOrNonThread(t *testing.T) {
	s := New(0)
	s.UpsertChannel(Channel{ID: 100, GuildID: 1, Kind: ChannelText})
	if s.SetArchived(999, true) {
		t.Error("SetArchived on unknown channel should return false")
	}
	if s.SetArchived(100, true) {
		t.Error("SetArchived on non-thread channel should return false")
	}
}

func TestRemoveThread(t *testing.T) {
	s := New(0)
	s.UpsertChannel(Channel{ID: 100, GuildID: 1, Kind: ChannelText})
	s.UpsertThread(thread(10, 100, time.Unix(1, 0), false))
	s.AppendMessage(Message{ChannelID: 10, Content: "hi"})
	s.RemoveThread(10)
	if _, ok := s.Channel(10); ok {
		t.Error("thread still present after RemoveThread")
	}
	if len(s.Messages(10)) != 0 {
		t.Error("thread messages not cleared after RemoveThread")
	}
	if len(s.Threads(100)) != 0 {
		t.Error("thread still listed after RemoveThread")
	}
	// Removing again is a safe no-op.
	s.RemoveThread(10)
}

func TestRemoveChannelCascadesThreadAndNotificationState(t *testing.T) {
	s := New(0)
	s.UpsertGuild(Guild{ID: 1, Name: "guild"})
	s.UpsertChannel(Channel{ID: 100, GuildID: 1, Kind: ChannelText})
	s.UpsertThread(thread(10, 100, time.Unix(1, 0), false))
	s.AppendMessage(Message{ID: 1, ChannelID: 100})
	s.AppendMessage(Message{ID: 2, ChannelID: 10})
	s.IncrementUnread(100)
	s.IncrementPing(100)

	s.RemoveChannel(100)

	if _, ok := s.Channel(100); ok {
		t.Fatal("parent channel remains after removal")
	}
	if _, ok := s.Channel(10); ok {
		t.Fatal("child thread remains after parent removal")
	}
	if s.Messages(100) != nil || s.Messages(10) != nil || s.Unread(100) != 0 || s.Pings(100) != 0 {
		t.Fatal("channel-owned state was not removed")
	}
	if got := s.GuildPings(1); got != 0 {
		t.Fatalf("guild pings after channel removal = %d, want 0", got)
	}
}

func TestRemoveGuildCascadesAllOwnedState(t *testing.T) {
	s := New(0)
	s.UpsertGuild(Guild{ID: 1, Name: "guild"})
	s.UpsertChannel(Channel{ID: 100, GuildID: 1, Kind: ChannelText})
	s.AppendMessage(Message{ID: 1, ChannelID: 100})
	s.UpsertMember(1, Member{ID: 2, Name: "member"})
	s.UpsertRole(1, Role{ID: 3, Name: "role"})
	s.SetGuildEmojis(1, []GuildEmoji{{ID: 4, Name: "emoji"}})
	s.SetGuildStickers(1, []GuildSticker{{ID: 5, Name: "sticker"}})
	s.SetGuildFolders([]GuildFolder{{ID: 6, GuildIDs: []GuildID{1}}})
	s.IncrementPing(100)

	s.RemoveGuild(1)

	if _, ok := s.Guild(1); ok || len(s.Channels(1)) != 0 || s.Messages(100) != nil ||
		len(s.Members(1)) != 0 || len(s.Roles(1)) != 0 || len(s.GuildEmojis(1)) != 0 ||
		len(s.GuildStickers(1)) != 0 || len(s.GuildFolders()) != 0 || s.GuildPings(1) != 0 {
		t.Fatal("guild-owned state was not fully removed")
	}
}

func TestSetGuildUnavailablePreservesGuildState(t *testing.T) {
	s := New(0)
	s.UpsertGuild(Guild{ID: 1, Name: "guild"})
	s.UpsertChannel(Channel{ID: 100, GuildID: 1})
	if !s.SetGuildUnavailable(1, true) {
		t.Fatal("known guild was not marked unavailable")
	}
	guild, ok := s.Guild(1)
	if !ok || !guild.Unavailable || len(s.Channels(1)) != 1 {
		t.Fatalf("unavailable guild state = %+v, channels=%v", guild, s.Channels(1))
	}
}

func TestSetThreadJoined(t *testing.T) {
	s := New(0)
	s.UpsertChannel(Channel{ID: 100, GuildID: 1, Kind: ChannelText})
	s.UpsertThread(thread(10, 100, time.Unix(1, 0), false))
	if !s.SetThreadJoined(10, true) {
		t.Fatal("SetThreadJoined returned false")
	}
	c, _ := s.Channel(10)
	if !c.Thread.Joined {
		t.Error("Joined not set")
	}
	if s.SetThreadJoined(999, true) {
		t.Error("SetThreadJoined on unknown thread should be false")
	}
}

func ids(cs []Channel) []ChannelID {
	out := make([]ChannelID, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}
