package store

import "testing"

func firstMessage(t *testing.T, s *Store, channel ChannelID) Message {
	t.Helper()
	msgs := s.Messages(channel)
	if len(msgs) == 0 {
		t.Fatalf("channel %d has no messages", channel)
	}
	return msgs[0]
}

// TestMessageRevBumpsOnEveryMutator is the test standing between this package
// and silently stale renders in the chat view.
//
// ChatView caches rendered lines per message and reuses them while Rev is
// unchanged. The cache cannot detect mutation any other way: Messages returns
// shallow copies whose Reactions and ComponentTree slices alias the ring's
// backing arrays, so an in-place patch mutates the cached copy too and every
// value comparison keeps reporting "unchanged".
//
// A mutator that forgets to stamp a fresh revision therefore renders stale
// output with no other symptom. If you add a message mutator, add it here.
func TestMessageRevBumpsOnEveryMutator(t *testing.T) {
	const channel ChannelID = 1
	const id MessageID = 100

	seed := func() *Store {
		s := New(0)
		s.AppendMessage(Message{
			ID:        id,
			ChannelID: channel,
			Nonce:     "nonce-1",
			Pending:   true,
			Content:   "hello",
			Reactions: []Reaction{{EmojiName: "👍", Count: 1}},
			ComponentTree: []ComponentNode{
				{Kind: ComponentButton, CustomID: "btn", Label: "Go"},
			},
		})
		return s
	}

	tests := []struct {
		name  string
		apply func(t *testing.T, s *Store)
	}{
		{
			name: "UpdateMessage",
			apply: func(t *testing.T, s *Store) {
				if !s.UpdateMessage(channel, id, func(m *Message) { m.Content = "edited" }) {
					t.Fatal("UpdateMessage reported no match")
				}
			},
		},
		{
			name: "SetMessagePinned",
			apply: func(t *testing.T, s *Store) {
				if !s.SetMessagePinned(channel, id, true) {
					t.Fatal("SetMessagePinned reported no match")
				}
			},
		},
		{
			// The canonical in-place patch: it writes through the ComponentTree
			// backing array that cached copies share.
			name: "SetComponentState",
			apply: func(t *testing.T, s *Store) {
				if !s.SetComponentState(channel, id, "btn", ComponentStatePending) {
					t.Fatal("SetComponentState reported no match")
				}
			},
		},
		{
			name: "AddReaction new entry",
			apply: func(t *testing.T, s *Store) {
				if !s.AddReaction(channel, id, Reaction{EmojiName: "🎉", Count: 1}) {
					t.Fatal("AddReaction reported no match")
				}
			},
		},
		{
			// Increments Count through the shared Reactions backing array.
			name: "AddReaction existing entry",
			apply: func(t *testing.T, s *Store) {
				if !s.AddReaction(channel, id, Reaction{EmojiName: "👍", Count: 1}) {
					t.Fatal("AddReaction reported no match")
				}
			},
		},
		{
			name: "RemoveReaction",
			apply: func(t *testing.T, s *Store) {
				if !s.RemoveReaction(channel, id, "👍", 0, false) {
					t.Fatal("RemoveReaction reported no match")
				}
			},
		},
		{
			name: "MarkFailed",
			apply: func(t *testing.T, s *Store) {
				if !s.MarkFailed(channel, "nonce-1") {
					t.Fatal("MarkFailed reported no match")
				}
			},
		},
		{
			name: "ReplaceMessage",
			apply: func(t *testing.T, s *Store) {
				ok := s.ReplaceMessage("nonce-1", Message{
					ID: id, ChannelID: channel, Content: "confirmed",
				})
				if !ok {
					t.Fatal("ReplaceMessage reported no match")
				}
			},
		},
		{
			name: "SetMessages",
			apply: func(t *testing.T, s *Store) {
				s.SetMessages(channel, []Message{{ID: id, Content: "replaced"}})
			},
		},
		{
			name: "AppendMessage",
			apply: func(t *testing.T, s *Store) {
				s.AppendMessage(Message{ID: id + 1, ChannelID: channel, Content: "next"})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := seed()
			before := firstMessage(t, s, channel).Rev()
			beforeChannel := s.MsgRev(channel)

			tc.apply(t, s)

			// Assert that *some* message carries a fresh revision, rather than
			// picking one positionally. SetMessages carries unconfirmed local
			// echoes over deliberately unstamped — they have not changed — so
			// the newest message is not always the mutated one.
			var after uint64
			for _, m := range s.Messages(channel) {
				if m.Rev() > after {
					after = m.Rev()
				}
			}
			if after <= before {
				t.Errorf("highest Rev = %d after %s, want > %d; a mutator that does not "+
					"stamp a fresh revision renders stale output undetectably",
					after, tc.name, before)
			}
			if got := s.MsgRev(channel); got <= beforeChannel {
				t.Errorf("MsgRev = %d after %s, want > %d", got, tc.name, beforeChannel)
			}
		})
	}
}

func TestMsgRevBumpsOnRemove(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 1})
	before := s.MsgRev(1)
	if !s.RemoveMessage(1, 1) {
		t.Fatal("RemoveMessage reported no match")
	}
	if got := s.MsgRev(1); got <= before {
		t.Fatalf("MsgRev = %d after remove, want > %d", got, before)
	}
}

func TestMsgsIntoReusesCapacity(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 1})
	s.AppendMessage(Message{ID: 2, ChannelID: 1})
	dst := make([]Message, 0, 2)
	base := &dst[:cap(dst)][0]
	got := s.MsgsInto(1, dst)
	if len(got) != 2 || &got[0] != base {
		t.Fatal("MsgsInto did not reuse destination capacity")
	}
}

// TestMessageRevNeverRepeats pins that revisions are monotonic across the whole
// store, not per channel. A per-ring counter would restart on SetMessages and
// could hand a message a revision it previously held with different content,
// which a cache would read as a hit.
func TestMessageRevNeverRepeats(t *testing.T) {
	s := New(0)
	seen := map[uint64]bool{}
	record := func(m Message) {
		if m.Rev() == 0 {
			t.Fatal("message was stored without a revision")
		}
		if seen[m.Rev()] {
			t.Fatalf("revision %d was reused", m.Rev())
		}
		seen[m.Rev()] = true
	}

	for i := 0; i < 5; i++ {
		s.AppendMessage(Message{ID: MessageID(i + 1), ChannelID: 1})
		s.AppendMessage(Message{ID: MessageID(i + 1), ChannelID: 2})
	}
	for _, channel := range []ChannelID{1, 2} {
		for _, m := range s.Messages(channel) {
			record(m)
		}
	}

	// Replacing channel 1's history must not reissue revisions already used.
	s.SetMessages(1, []Message{{ID: 99}, {ID: 98}})
	for _, m := range s.Messages(1) {
		record(m)
	}
}

// TestMessageRevSurvivesTheShallowCopy pins the property that makes rev usable
// as a snapshot: it is a scalar, so a copy taken from Messages keeps the old
// value even when the store patches the message in place afterwards. The
// message's own slices do not behave this way, which is the whole reason rev
// exists.
func TestMessageRevSurvivesTheShallowCopy(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{
		ID: 1, ChannelID: 1,
		ComponentTree: []ComponentNode{{Kind: ComponentButton, CustomID: "btn"}},
	})
	snapshot := firstMessage(t, s, 1)

	s.SetComponentState(1, 1, "btn", ComponentStatePending)

	if got := firstMessage(t, s, 1).Rev(); got == snapshot.Rev() {
		t.Fatal("Rev did not change across an in-place mutation")
	}
	// The aliasing hazard itself: the snapshot's tree was mutated underneath it,
	// so only the revision distinguishes the versions.
	if snapshot.ComponentTree[0].State != ComponentStatePending {
		t.Skip("ComponentTree no longer aliases the ring; rev may be redundant")
	}
}

func TestMetaRevBumpsOnNonMessageMutations(t *testing.T) {
	tests := []struct {
		name  string
		apply func(s *Store)
	}{
		{"UpsertMember", func(s *Store) { s.UpsertMember(1, Member{ID: 42, Name: "alice"}) }},
		{"RemoveMember", func(s *Store) { s.RemoveMember(1, 42) }},
		{"UpsertRole", func(s *Store) { s.UpsertRole(1, Role{ID: 7, Name: "mod"}) }},
		{"RemoveRole", func(s *Store) { s.RemoveRole(1, 7) }},
		{"UpsertChannel", func(s *Store) { s.UpsertChannel(Channel{ID: 5, GuildID: 1, Name: "general"}) }},
		{"UpsertGuild", func(s *Store) { s.UpsertGuild(Guild{ID: 1, Name: "guild"}) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := New(0)
			before := s.MetaRev()
			tc.apply(s)
			if s.MetaRev() <= before {
				t.Errorf("MetaRev = %d, want > %d; mention resolution reads this state, "+
					"so a render cached before it changed must be invalidated",
					s.MetaRev(), before)
			}
		})
	}
}

// TestMetaRevIgnoresMessageMutations keeps the two counters independent: a
// message edit must not invalidate every other message's cached render.
func TestMetaRevIgnoresMessageMutations(t *testing.T) {
	s := New(0)
	s.AppendMessage(Message{ID: 1, ChannelID: 1, Content: "hi"})
	before := s.MetaRev()

	s.AppendMessage(Message{ID: 2, ChannelID: 1, Content: "there"})
	s.UpdateMessage(1, 1, func(m *Message) { m.Content = "edited" })
	s.AddReaction(1, 1, Reaction{EmojiName: "👍", Count: 1})

	if s.MetaRev() != before {
		t.Errorf("MetaRev = %d, want it unchanged at %d: message mutations must not "+
			"invalidate renders of other messages", s.MetaRev(), before)
	}
}

// TestMetaRevStableOnUnchangedMemberUpserts guards the hot path: every guild
// MESSAGE_CREATE re-upserts its author's member, so an unchanged re-upsert must
// not bump MetaRev (which would invalidate the whole transcript cache).
func TestMetaRevStableOnUnchangedMemberUpserts(t *testing.T) {
	s := New(0)
	member := Member{ID: 42, Name: "alice", Username: "alice", RoleIDs: []RoleID{7, 8}}
	s.UpsertMember(1, member)
	before := s.MetaRev()

	// Identical re-upsert (same identity and roles) changes nothing.
	s.UpsertMember(1, Member{ID: 42, Name: "alice", Username: "alice", RoleIDs: []RoleID{7, 8}})
	// RememberMemberIdentity with the same/global-only identity changes nothing.
	s.RememberMemberIdentity(1, Member{ID: 42, Username: "alice"})
	if s.MetaRev() != before {
		t.Fatalf("MetaRev = %d, want unchanged at %d after no-op member upserts", s.MetaRev(), before)
	}

	// A real change (added role) must still invalidate.
	s.UpsertMember(1, Member{ID: 42, Name: "alice", Username: "alice", RoleIDs: []RoleID{7, 8, 9}})
	if s.MetaRev() == before {
		t.Fatal("MetaRev did not advance on a changed member")
	}
}
