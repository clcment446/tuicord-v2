package store

// UpdateMessage finds the message with id in channel's ring and applies patch
// to it in place. It returns true when a matching message was found.
//
// The primary use-case is MESSAGE_UPDATE events, which Discord sends after
// MESSAGE_CREATE once link embeds are unfurled.
// It is the single chokepoint through which SetMessagePinned and
// SetComponentState also mutate, so stamping a fresh revision here covers them.
func (s *Store) UpdateMessage(channel ChannelID, id MessageID, patch func(*Message)) bool {
	r := s.messages[channel]
	if r == nil {
		return false
	}
	return r.updateByID(id, patch, s.nextRevision())
}

// RemoveMessage deletes message id from channel's ring. It returns true when a
// matching message was present.
func (s *Store) RemoveMessage(channel ChannelID, id MessageID) bool {
	if id == 0 {
		return false
	}
	deleted := s.deletedMessages[channel]
	if deleted == nil {
		deleted = make(map[MessageID]uint64)
		s.deletedMessages[channel] = deleted
	}
	deleted[id] = s.nextRevision()
	r := s.messages[channel]
	if r == nil {
		return false
	}
	return r.removeByID(id)
}

// SetMessagePinned patches a message's cached pin state. Discord's
// CHANNEL_PINS_UPDATE event does not include the message ID, so successful REST
// pin/unpin calls use this helper to keep the visible row accurate.
func (s *Store) SetMessagePinned(channel ChannelID, id MessageID, pinned bool) bool {
	return s.UpdateMessage(channel, id, func(m *Message) {
		m.Pinned = pinned
	})
}

// SetComponentState patches the local UI state of every component whose
// CustomID matches on message id in channel, recursing through the Components
// V2 tree. It backs the pending/success/error feedback drawn on interactive
// controls while a component interaction is in flight.
func (s *Store) SetComponentState(channel ChannelID, id MessageID, customID string, state ComponentState) bool {
	if customID == "" {
		return false
	}
	return s.UpdateMessage(channel, id, func(m *Message) {
		setComponentNodeState(m.ComponentTree, customID, state)
	})
}

func setComponentNodeState(nodes []ComponentNode, customID string, state ComponentState) {
	for i := range nodes {
		if nodes[i].CustomID == customID {
			nodes[i].State = state
		}
		if acc := nodes[i].Accessory; acc != nil && acc.CustomID == customID {
			acc.State = state
		}
		setComponentNodeState(nodes[i].Children, customID, state)
	}
}

// AddReaction merges r into the Reactions slice of message id in channel.
// If a reaction entry with the same EmojiName and EmojiID already exists its
// Count is incremented and Me is set when r.Me is true; otherwise r is
// appended as a new entry. Returns true when the message was found.
func (s *Store) AddReaction(channel ChannelID, id MessageID, r Reaction) bool {
	ring := s.messages[channel]
	if ring == nil {
		return false
	}
	return ring.addReaction(id, r, s.nextRevision())
}

// RemoveReaction decrements the reaction identified by emojiName and emojiID
// on message id in channel. When me is true the Me flag is cleared. The
// reaction entry is removed once its Count reaches zero. Returns true when the
// message and a matching reaction entry were both found.
func (s *Store) RemoveReaction(channel ChannelID, id MessageID, emojiName string, emojiID uint64, me bool) bool {
	ring := s.messages[channel]
	if ring == nil {
		return false
	}
	return ring.removeReaction(id, emojiName, emojiID, me, s.nextRevision())
}

// MemberColor returns the effective display color for user in guild following
// Discord's rule: the color of the highest-Position role among the member's
// RoleIDs that has a non-zero Color. Returns 0 when the member is unknown or
// none of their roles carry a color.
//
// Position ties are broken deterministically by RoleID (lower ID wins).
func (s *Store) MemberColor(guild GuildID, user UserID) uint32 {
	role, ok := s.MemberDisplayRole(guild, user)
	if !ok {
		return 0
	}
	return role.GradientAt(0)
}

// MemberDisplayRole returns the highest-position role that gives user a
// visible display color. It includes gradient-only roles, whose flat Color is
// intentionally zero in Discord's payload.
func (s *Store) MemberDisplayRole(guild GuildID, user UserID) (Role, bool) {
	m, ok := s.members[guild][user]
	if !ok {
		return Role{}, false
	}
	roles := s.roles[guild]
	bestPos := -1
	bestID := RoleID(0)
	var best Role
	found := false
	for _, rid := range m.RoleIDs {
		r, exists := roles[rid]
		if !exists || r.GradientAt(0) == 0 {
			continue
		}
		higher := r.Position > bestPos
		tied := r.Position == bestPos && (!found || rid < bestID)
		if higher || tied {
			bestPos = r.Position
			bestID = rid
			best = r
			found = true
		}
	}
	return best, found
}
