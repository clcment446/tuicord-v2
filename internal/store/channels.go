package store

// This file holds the channel-level helpers that go beyond the flat
// guild→channel→messages model: threads (channels parented to a channel) and
// the channel-overwrite permission calculation. The sort/transition logic is
// kept small and pure so the thread lifecycle (create → archive → unarchive →
// delete) and overwrite folding are table-testable.

// UpsertThread inserts or updates a thread channel. It is a thin, intent-named
// wrapper over [Store.UpsertChannel]: threads are ordinary channels carrying a
// [ThreadMeta] and a ParentID pointing at their text/announcement/forum parent.
func (s *Store) UpsertThread(c Channel) {
	if c.Thread == nil {
		c.Thread = &ThreadMeta{}
	}
	c.Kind = ChannelThread
	s.UpsertChannel(c)
}

// RemoveThread deletes a thread channel and its cached messages. It is safe to
// call for an unknown id.
func (s *Store) RemoveThread(id ChannelID) {
	s.removeChannel(id)
}

// removeChannel drops a channel from the store: its record, its guild ordering
// slot, and any cached message ring.
func (s *Store) removeChannel(id ChannelID) {
	c, ok := s.channels[id]
	if !ok {
		return
	}
	order := s.channelOrder[c.GuildID]
	for i, cid := range order {
		if cid == id {
			s.channelOrder[c.GuildID] = append(order[:i], order[i+1:]...)
			break
		}
	}
	delete(s.channels, id)
	delete(s.messages, id)
	delete(s.unread, id)
}

// SetArchived flips a thread's archived state, returning true when the channel
// existed and was a thread. Unarchiving a thread that was never archived, or
// archiving one already archived, is a no-op that still reports true.
func (s *Store) SetArchived(id ChannelID, archived bool) bool {
	c, ok := s.channels[id]
	if !ok || c.Thread == nil {
		return false
	}
	meta := *c.Thread
	meta.Archived = archived
	c.Thread = &meta
	s.channels[id] = c
	return true
}

// SetThreadJoined records whether the logged-in account is a member of a
// thread, as delivered by THREAD_MEMBER_UPDATE. It reports whether the thread
// was known.
func (s *Store) SetThreadJoined(id ChannelID, joined bool) bool {
	c, ok := s.channels[id]
	if !ok || c.Thread == nil {
		return false
	}
	meta := *c.Thread
	meta.Joined = joined
	c.Thread = &meta
	s.channels[id] = c
	return true
}

// Threads returns the active (non-archived) threads parented to parent, sorted
// by most recent activity first. Archived threads are excluded; use
// [Store.ArchivedThreads] for those. Ties on LastActive break by descending ID
// (newer snowflakes first), matching Discord's ordering.
func (s *Store) Threads(parent ChannelID) []Channel {
	return s.threadsBy(parent, false)
}

// ArchivedThreads returns the archived threads parented to parent, sorted the
// same way as [Store.Threads].
func (s *Store) ArchivedThreads(parent ChannelID) []Channel {
	return s.threadsBy(parent, true)
}

func (s *Store) threadsBy(parent ChannelID, archived bool) []Channel {
	var out []Channel
	for _, id := range s.channelOrder[parentGuild(s, parent)] {
		c := s.channels[id]
		if c.Kind != ChannelThread || c.ParentID != parent || c.Thread == nil {
			continue
		}
		if c.Thread.Archived != archived {
			continue
		}
		out = append(out, c)
	}
	sortThreads(out)
	return out
}

// parentGuild resolves the guild a parent channel belongs to so thread scanning
// only walks that guild's channel order. Falls back to scanning every guild
// when the parent is unknown (e.g. threads that arrived before their parent).
func parentGuild(s *Store, parent ChannelID) GuildID {
	if c, ok := s.channels[parent]; ok {
		return c.GuildID
	}
	// Parent unknown: find the guild from any thread pointing at it.
	for _, c := range s.channels {
		if c.ParentID == parent && c.Kind == ChannelThread {
			return c.GuildID
		}
	}
	return 0
}

func sortThreads(ts []Channel) {
	for i := 1; i < len(ts); i++ {
		for j := i; j > 0 && lessThread(ts[j], ts[j-1]); j-- {
			ts[j], ts[j-1] = ts[j-1], ts[j]
		}
	}
}

// lessThread orders threads most-recent-activity first, breaking ties by
// descending ID so newer threads sort ahead of older ones.
func lessThread(a, b Channel) bool {
	at, bt := a.Thread.LastActive, b.Thread.LastActive
	if !at.Equal(bt) {
		return at.After(bt)
	}
	return a.ID > b.ID
}
