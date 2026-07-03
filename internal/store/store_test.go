package store

import "testing"

func TestGuildsPreserveFirstSeenOrder(t *testing.T) {
	s := New(0)
	s.UpsertGuild(Guild{ID: 3, Name: "c"})
	s.UpsertGuild(Guild{ID: 1, Name: "a"})
	s.UpsertGuild(Guild{ID: 3, Name: "c2"}) // update, not reorder

	got := s.Guilds()
	if len(got) != 2 {
		t.Fatalf("want 2 guilds, got %d", len(got))
	}
	if got[0].ID != 3 || got[0].Name != "c2" {
		t.Errorf("first guild = %+v, want id 3 name c2", got[0])
	}
	if got[1].ID != 1 {
		t.Errorf("second guild id = %d, want 1", got[1].ID)
	}
}

func TestChannelsSortedByPositionThenID(t *testing.T) {
	s := New(0)
	s.UpsertChannel(Channel{ID: 20, GuildID: 1, Name: "b", Position: 1})
	s.UpsertChannel(Channel{ID: 10, GuildID: 1, Name: "a", Position: 0})
	s.UpsertChannel(Channel{ID: 5, GuildID: 1, Name: "c", Position: 1})

	got := s.Channels(1)
	wantIDs := []ChannelID{10, 5, 20}
	if len(got) != len(wantIDs) {
		t.Fatalf("want %d channels, got %d", len(wantIDs), len(got))
	}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Errorf("channel[%d].ID = %d, want %d", i, got[i].ID, id)
		}
	}
}

func TestAppendMessageEvictsOldest(t *testing.T) {
	s := New(2)
	s.AppendMessage(Message{ID: 1, ChannelID: 7, Content: "one"})
	s.AppendMessage(Message{ID: 2, ChannelID: 7, Content: "two"})
	s.AppendMessage(Message{ID: 3, ChannelID: 7, Content: "three"})

	got := s.Messages(7)
	if len(got) != 2 {
		t.Fatalf("want 2 messages after eviction, got %d", len(got))
	}
	if got[0].ID != 2 || got[1].ID != 3 {
		t.Errorf("messages = %d,%d; want 2,3", got[0].ID, got[1].ID)
	}
}

func TestReplaceMessageByNonce(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ChannelID: 7, Content: "hi", Nonce: "abc", Pending: true})

	ok := s.ReplaceMessage("abc", Message{ID: 99, ChannelID: 7, Content: "hi", Nonce: "abc"})
	if !ok {
		t.Fatal("ReplaceMessage returned false for known nonce")
	}
	got := s.Messages(7)
	if len(got) != 1 || got[0].ID != 99 || got[0].Pending {
		t.Errorf("after replace = %+v, want id 99 not pending", got[0])
	}

	if s.ReplaceMessage("missing", Message{ChannelID: 7}) {
		t.Error("ReplaceMessage returned true for unknown nonce")
	}
	if s.ReplaceMessage("", Message{ChannelID: 7}) {
		t.Error("ReplaceMessage returned true for empty nonce")
	}
}

func TestMarkFailed(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ChannelID: 7, Nonce: "n1", Pending: true})

	if !s.MarkFailed(7, "n1") {
		t.Fatal("MarkFailed returned false for known nonce")
	}
	got := s.Messages(7)[0]
	if !got.Failed || got.Pending {
		t.Errorf("after MarkFailed = %+v, want failed and not pending", got)
	}
	if s.MarkFailed(7, "nope") {
		t.Error("MarkFailed returned true for unknown nonce")
	}
	if s.MarkFailed(999, "n1") {
		t.Error("MarkFailed returned true for unknown channel")
	}
}

func TestMemberAndChannelResolution(t *testing.T) {
	s := New(0)
	s.UpsertMember(1, Member{ID: 42, Name: "alice"})
	s.UpsertChannel(Channel{ID: 5, GuildID: 1, Name: "general"})

	if name, ok := s.MemberName(1, 42); !ok || name != "alice" {
		t.Errorf("MemberName = %q,%v; want alice,true", name, ok)
	}
	if _, ok := s.MemberName(1, 999); ok {
		t.Error("MemberName ok for unknown user")
	}
	if name, ok := s.ChannelName(5); !ok || name != "general" {
		t.Errorf("ChannelName = %q,%v; want general,true", name, ok)
	}
	if _, ok := s.ChannelName(999); ok {
		t.Error("ChannelName ok for unknown channel")
	}
}

func TestMessagesUnknownChannelIsNil(t *testing.T) {
	s := New(0)
	if got := s.Messages(123); got != nil {
		t.Errorf("Messages(unknown) = %v, want nil", got)
	}
}
