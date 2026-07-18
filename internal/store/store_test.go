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

func TestUpsertGuildKeepsNameWhenUpdateIsEmpty(t *testing.T) {
	s := New(0)
	s.UpsertGuild(Guild{ID: 1, Name: "gophers"})
	s.UpsertGuild(Guild{ID: 1})

	if name, ok := s.GuildName(1); !ok || name != "gophers" {
		t.Fatalf("GuildName = %q,%v; want gophers,true", name, ok)
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
	s.UpsertGuild(Guild{ID: 1, Name: "gophers"})
	s.UpsertMember(1, Member{ID: 42, Name: "alice", RoleIDs: []RoleID{10, 11}})
	s.UpsertRole(1, Role{ID: 10, Name: "admin", Position: 10, Hoist: true})
	s.UpsertRole(1, Role{ID: 11, Name: "member", Position: 1})
	s.UpsertChannel(Channel{ID: 5, GuildID: 1, Name: "general"})

	if name, ok := s.GuildName(1); !ok || name != "gophers" {
		t.Errorf("GuildName = %q,%v; want gophers,true", name, ok)
	}
	if _, ok := s.GuildName(999); ok {
		t.Error("GuildName ok for unknown guild")
	}
	if name, ok := s.MemberName(1, 42); !ok || name != "alice" {
		t.Errorf("MemberName = %q,%v; want alice,true", name, ok)
	}
	if member, ok := s.Member(1, 42); !ok || len(member.RoleIDs) != 2 || member.RoleIDs[0] != 10 {
		t.Errorf("Member = %+v,%v; want alice with role IDs", member, ok)
	}
	if _, ok := s.MemberName(1, 999); ok {
		t.Error("MemberName ok for unknown user")
	}
	if role, ok := s.Role(1, 10); !ok || role.Name != "admin" || !role.Hoist {
		t.Errorf("Role = %+v,%v; want admin,true", role, ok)
	}
	roles := s.Roles(1)
	if len(roles) != 2 || roles[0].ID != 10 || roles[1].ID != 11 {
		t.Errorf("Roles = %+v; want position-sorted admin,member", roles)
	}
	if name, ok := s.ChannelName(5); !ok || name != "general" {
		t.Errorf("ChannelName = %q,%v; want general,true", name, ok)
	}
	if _, ok := s.ChannelName(999); ok {
		t.Error("ChannelName ok for unknown channel")
	}
}

func TestRememberMemberIdentityPreservesCachedRoleData(t *testing.T) {
	s := New(0)
	s.UpsertMember(1, Member{ID: 42, Name: "old", Nick: "ali", AvatarURL: "guild-avatar", RoleIDs: []RoleID{7}})
	s.RememberMemberIdentity(1, Member{ID: 42, Name: "Alice", Username: "alice", AvatarURL: "global-avatar"})

	got, ok := s.Member(1, 42)
	if !ok || got.Name != "old" || got.Username != "alice" || got.Nick != "ali" || got.AvatarURL != "guild-avatar" || len(got.RoleIDs) != 1 || got.RoleIDs[0] != 7 {
		t.Fatalf("remembered member = %+v, %t", got, ok)
	}
}

func TestChannelRecipientResolution(t *testing.T) {
	s := New(0)
	s.UpsertChannel(Channel{ID: 90, GuildID: ^GuildID(0), Name: "alice", Kind: ChannelDM,
		Recipients: []Member{{ID: 42, Name: "Alice"}}})

	member, ok := s.ChannelRecipient(90, 42)
	if !ok || member.Name != "Alice" {
		t.Fatalf("ChannelRecipient = %+v,%v; want Alice,true", member, ok)
	}
	if _, ok := s.ChannelRecipient(90, 99); ok {
		t.Fatal("ChannelRecipient resolved an unknown user")
	}
}

func TestMessagesUnknownChannelIsNil(t *testing.T) {
	s := New(0)
	if got := s.Messages(123); got != nil {
		t.Errorf("Messages(unknown) = %v, want nil", got)
	}
}

func TestSetMessagesReplacesHistoryOldestFirst(t *testing.T) {
	s := New(2)
	s.AppendMessage(Message{ID: 1, ChannelID: 7, Content: "old"})

	s.SetMessages(7, []Message{
		{ID: 2, Content: "two"},
		{ID: 3, Content: "three"},
		{ID: 4, Content: "four"},
	})

	got := s.Messages(7)
	if len(got) != 2 {
		t.Fatalf("want 2 messages after bounded replace, got %d", len(got))
	}
	if got[0].ID != 3 || got[1].ID != 4 {
		t.Fatalf("messages = %+v, want ids 3,4 oldest-first", got)
	}
	if got[0].ChannelID != 7 || got[1].ChannelID != 7 {
		t.Fatalf("messages channel ids = %d,%d; want 7,7", got[0].ChannelID, got[1].ChannelID)
	}
}

func TestPrependMessagesKeepsOlderHistoryFirst(t *testing.T) {
	s := New(0)
	s.SetMessages(7, []Message{{ID: 3}, {ID: 4}})
	s.PrependMessages(7, []Message{{ID: 1}, {ID: 2}})

	got := s.Messages(7)
	if len(got) != 4 || got[0].ID != 1 || got[1].ID != 2 || got[2].ID != 3 || got[3].ID != 4 {
		t.Fatalf("messages = %+v, want IDs 1,2,3,4", got)
	}
}

func TestPrependMessagesAtCapacityRetainsFetchedOlderPage(t *testing.T) {
	s := New(4)
	s.SetMessages(7, []Message{{ID: 5}, {ID: 6}, {ID: 7}, {ID: 8}})

	s.PrependMessages(7, []Message{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}})

	got := s.Messages(7)
	if ids := messageIDs(got); len(ids) != 4 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 || ids[3] != 4 {
		t.Fatalf("messages = %v, want fetched IDs [1 2 3 4]", ids)
	}
}

func TestPrependMessagesSinceRetainsPostRequestArrivalAtCapacity(t *testing.T) {
	s := New(4)
	s.SetMessages(7, []Message{{ID: 5}, {ID: 6}, {ID: 7}, {ID: 8}})
	requestRevision := s.Revision()
	s.AppendMessage(Message{ID: 9, ChannelID: 7})

	s.PrependMessagesSince(7, []Message{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}, requestRevision)

	got := messageIDs(s.Messages(7))
	want := []MessageID{1, 2, 3, 9}
	if len(got) != len(want) {
		t.Fatalf("messages = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("messages = %v, want %v", got, want)
		}
	}
}

func TestDeleteTombstoneSurvivesMissingRingUntilExplicitCreate(t *testing.T) {
	s := New(0)
	if s.RemoveMessage(7, 99) {
		t.Fatal("delete of uncached message reported a removal")
	}
	if !s.MessageTombstoned(7, 99) {
		t.Fatal("uncached delete did not leave a tombstone")
	}
	s.AppendMessage(Message{ID: 99, ChannelID: 7})
	if s.MessageTombstoned(7, 99) {
		t.Fatal("explicit create did not clear tombstone")
	}
}

func TestPrependMessagesDeduplicatesOverlapAndPreservesOrderAtCapacity(t *testing.T) {
	s := New(5)
	s.SetMessages(7, []Message{{ID: 4}, {ID: 5}, {ID: 6}, {ID: 7}, {ID: 8}})

	s.PrependMessages(7, []Message{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 4}})

	got := messageIDs(s.Messages(7))
	want := []MessageID{1, 2, 3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("messages = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("messages = %v, want %v", got, want)
		}
	}
}

func messageIDs(messages []Message) []MessageID {
	ids := make([]MessageID, len(messages))
	for i, message := range messages {
		ids[i] = message.ID
	}
	return ids
}
