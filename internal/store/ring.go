package store

// ring is a fixed-capacity FIFO buffer of messages. When full, pushing evicts
// the oldest message. It keeps message history bounded per channel.
type ring struct {
	buf   []Message
	start int // index of the oldest element
	size  int
}

func newRing(capacity int) *ring {
	if capacity < 1 {
		capacity = 1
	}
	return &ring{buf: make([]Message, capacity)}
}

func (r *ring) push(m Message) {
	if r.size < len(r.buf) {
		r.buf[(r.start+r.size)%len(r.buf)] = m
		r.size++
		return
	}
	// Full: overwrite the oldest and advance start.
	r.buf[r.start] = m
	r.start = (r.start + 1) % len(r.buf)
}

// slice returns the messages oldest-first as a fresh copy.
func (r *ring) slice() []Message {
	out := make([]Message, r.size)
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(r.start+i)%len(r.buf)]
	}
	return out
}

func (r *ring) replaceByNonce(nonce string, confirmed Message) bool {
	if i, ok := r.indexByNonce(nonce); ok {
		r.buf[i] = confirmed
		return true
	}
	return false
}

func (r *ring) markFailed(nonce string) bool {
	if i, ok := r.indexByNonce(nonce); ok {
		r.buf[i].Failed = true
		r.buf[i].Pending = false
		return true
	}
	return false
}

func (r *ring) indexByNonce(nonce string) (int, bool) {
	for i := 0; i < r.size; i++ {
		idx := (r.start + i) % len(r.buf)
		if r.buf[idx].Nonce == nonce {
			return idx, true
		}
	}
	return 0, false
}

// indexByID returns the physical buffer index of the message with the given ID,
// searching from oldest to newest. Returns (0, false) when not found.
func (r *ring) indexByID(id MessageID) (int, bool) {
	for i := 0; i < r.size; i++ {
		idx := (r.start + i) % len(r.buf)
		if r.buf[idx].ID == id {
			return idx, true
		}
	}
	return 0, false
}

// updateByID applies patch to the message with id. It follows the same
// in-place mutation style as markFailed.
func (r *ring) updateByID(id MessageID, patch func(*Message)) bool {
	if i, ok := r.indexByID(id); ok {
		patch(&r.buf[i])
		return true
	}
	return false
}

func (r *ring) removeByID(id MessageID) bool {
	i, ok := r.indexByID(id)
	if !ok {
		return false
	}
	for n := 0; n < r.size-1; n++ {
		from := (i + 1 + n) % len(r.buf)
		to := (i + n) % len(r.buf)
		r.buf[to] = r.buf[from]
	}
	last := (r.start + r.size - 1) % len(r.buf)
	r.buf[last] = Message{}
	r.size--
	if r.size == 0 {
		r.start = 0
	}
	return true
}

// addReaction merges react into the message with id. If a matching entry
// (same EmojiName and EmojiID) already exists its Count is incremented and Me
// is set when react.Me is true; otherwise react is appended as a new entry.
func (r *ring) addReaction(id MessageID, react Reaction) bool {
	i, ok := r.indexByID(id)
	if !ok {
		return false
	}
	msg := &r.buf[i]
	for j := range msg.Reactions {
		rx := &msg.Reactions[j]
		if rx.EmojiName == react.EmojiName && rx.EmojiID == react.EmojiID {
			rx.Count++
			if react.Me {
				rx.Me = true
			}
			return true
		}
	}
	msg.Reactions = append(msg.Reactions, react)
	return true
}

// removeReaction decrements the matching reaction on message id. When me is
// true, the Me flag is cleared. The entry is removed once Count reaches zero.
func (r *ring) removeReaction(id MessageID, emojiName string, emojiID uint64, me bool) bool {
	i, ok := r.indexByID(id)
	if !ok {
		return false
	}
	msg := &r.buf[i]
	for j := range msg.Reactions {
		rx := &msg.Reactions[j]
		if rx.EmojiName != emojiName || rx.EmojiID != emojiID {
			continue
		}
		if me {
			rx.Me = false
		}
		rx.Count--
		if rx.Count <= 0 {
			msg.Reactions = append(msg.Reactions[:j], msg.Reactions[j+1:]...)
		}
		return true
	}
	return false
}
