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
	s.RemoveChannel(id)
}

// RemoveChannel drops a channel and any child threads from the store, including
// cached messages and notification state. It is safe to call for an unknown ID.
func (s *Store) RemoveChannel(id ChannelID) {
	for childID, child := range s.channels {
		if child.Kind == ChannelThread && child.ParentID == id {
			s.removeChannel(childID)
		}
	}
	if !s.removeChannel(id) {
		// A delete can race channel hydration. Still clear any message-only state
		// and advance the lifetime so older async responses cannot recreate it.
		delete(s.messages, id)
		delete(s.deletedMessages, id)
		delete(s.prunedDeleteRevision, id)
		delete(s.latestMessage, id)
		delete(s.unread, id)
		s.clearPing(id)
		s.channelGeneration[id]++
		s.touchMeta()
	}
}

// RemoveGuild deletes a guild and all state owned by it. Even an unknown guild
// advances its generation so an older directory request cannot recreate it.
func (s *Store) RemoveGuild(id GuildID) {
	for _, channelID := range append([]ChannelID(nil), s.channelOrder[id]...) {
		s.removeChannel(channelID)
	}
	delete(s.channelOrder, id)
	delete(s.members, id)
	delete(s.roles, id)
	delete(s.guildEmojis, id)
	delete(s.guildStickers, id)
	delete(s.guildPings, id)
	delete(s.guilds, id)
	s.guildGeneration[id]++
	for i, guildID := range s.guildOrder {
		if guildID == id {
			s.guildOrder = append(s.guildOrder[:i], s.guildOrder[i+1:]...)
			break
		}
	}
	for i := len(s.guildFolders) - 1; i >= 0; i-- {
		folder := &s.guildFolders[i]
		for j := len(folder.GuildIDs) - 1; j >= 0; j-- {
			if folder.GuildIDs[j] == id {
				folder.GuildIDs = append(folder.GuildIDs[:j], folder.GuildIDs[j+1:]...)
			}
		}
		if len(folder.GuildIDs) == 0 {
			s.guildFolders = append(s.guildFolders[:i], s.guildFolders[i+1:]...)
		}
	}
	s.touchMeta()
}

// SetGuildUnavailable records a temporary outage without discarding cached
// guild state. It reports whether the guild was known.
func (s *Store) SetGuildUnavailable(id GuildID, unavailable bool) bool {
	guild, ok := s.guilds[id]
	if !ok {
		return false
	}
	guild.Unavailable = unavailable
	s.guilds[id] = guild
	s.touchMeta()
	return true
}

// removeChannel drops one channel from the store.
func (s *Store) removeChannel(id ChannelID) bool {
	c, ok := s.channels[id]
	if !ok {
		return false
	}
	s.removeChannelOrder(c.GuildID, id)
	// clearPing needs the channel's guild before the directory entry disappears.
	s.clearPing(id)
	delete(s.channels, id)
	delete(s.messages, id)
	delete(s.deletedMessages, id)
	delete(s.prunedDeleteRevision, id)
	delete(s.latestMessage, id)
	delete(s.unread, id)
	s.channelGeneration[id]++
	s.touchMeta()
	return true
}

func (s *Store) removeChannelOrder(guild GuildID, id ChannelID) {
	order := s.channelOrder[guild]
	for i, channelID := range order {
		if channelID == id {
			s.channelOrder[guild] = append(order[:i], order[i+1:]...)
			return
		}
	}
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
	s.touchMeta()
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
	s.touchMeta()
	return true
}

// SyncActiveThreads applies Discord's authoritative THREAD_LIST_SYNC semantics.
// A nil parents slice covers the whole guild; a non-nil slice covers only those
// parent channel IDs. Cached active threads absent from incoming are removed,
// while archived and out-of-scope threads remain untouched. Removed IDs are
// returned so App can invalidate their async resource gates.
func (s *Store) SyncActiveThreads(guild GuildID, parents []ChannelID, incoming []Channel) []ChannelID {
	present := make(map[ChannelID]struct{}, len(incoming))
	for _, thread := range incoming {
		present[thread.ID] = struct{}{}
	}
	var scope map[ChannelID]struct{}
	if parents != nil {
		scope = make(map[ChannelID]struct{}, len(parents))
		for _, parent := range parents {
			scope[parent] = struct{}{}
		}
	}
	var removed []ChannelID
	for _, id := range append([]ChannelID(nil), s.channelOrder[guild]...) {
		thread := s.channels[id]
		if thread.Kind != ChannelThread || thread.Thread == nil || thread.Thread.Archived {
			continue
		}
		if scope != nil {
			if _, ok := scope[thread.ParentID]; !ok {
				continue
			}
		}
		if _, ok := present[id]; ok {
			continue
		}
		s.RemoveThread(id)
		removed = append(removed, id)
	}
	for _, thread := range incoming {
		s.UpsertThread(thread)
	}
	return removed
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
