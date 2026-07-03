package store

// UpdateMessage finds the message with id in channel's ring and applies patch
// to it in place. It returns true when a matching message was found.
//
// The primary use-case is MESSAGE_UPDATE events, which Discord sends after
// MESSAGE_CREATE once link embeds are unfurled.
func (s *Store) UpdateMessage(channel ChannelID, id MessageID, patch func(*Message)) bool {
	r := s.messages[channel]
	if r == nil {
		return false
	}
	return r.updateByID(id, patch)
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
	return ring.addReaction(id, r)
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
	return ring.removeReaction(id, emojiName, emojiID, me)
}

// MemberColor returns the effective display color for user in guild following
// Discord's rule: the color of the highest-Position role among the member's
// RoleIDs that has a non-zero Color. Returns 0 when the member is unknown or
// none of their roles carry a color.
//
// Position ties are broken deterministically by RoleID (lower ID wins).
func (s *Store) MemberColor(guild GuildID, user UserID) uint32 {
	m, ok := s.members[guild][user]
	if !ok {
		return 0
	}
	roles := s.roles[guild]
	bestPos := -1
	bestID := RoleID(0)
	var bestColor uint32
	for _, rid := range m.RoleIDs {
		r, exists := roles[rid]
		if !exists || r.Color == 0 {
			continue
		}
		higher := r.Position > bestPos
		tied := r.Position == bestPos && (bestColor == 0 || rid < bestID)
		if higher || tied {
			bestPos = r.Position
			bestID = rid
			bestColor = r.Color
		}
	}
	return bestColor
}
